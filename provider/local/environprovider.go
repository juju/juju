// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.provider.local")

var _ environs.EnvironProvider = (*environProvider)(nil)

type environProvider struct{}

var providerInstance = &environProvider{}

var userCurrent = user.Current

// Open implements environs.EnvironProvider.Open.
func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Infof("opening environment %q", cfg.Name())
	// Do the initial validation on the config.
	localConfig, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := VerifyPrerequisites(localConfig.container()); err != nil {
		return nil, errors.Annotate(err, "failed verification of local provider prerequisites")
	}
	if cfg, err = providerInstance.correctLocalhostURLs(cfg, localConfig); err != nil {
		return nil, errors.Annotate(err, "failed to replace localhost references in loopback URLs specified in proxy config settings")
	}
	environ := &localEnviron{name: cfg.Name()}
	if err := environ.SetConfig(cfg); err != nil {
		return nil, errors.Annotate(err, "failure setting config")
	}
	return environ, nil
}

// Schema returns the configuration schema for an environment.
func (environProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

// correctLocalhostURLs exams proxy attributes and changes URL values pointing to localhost to use bridge IP.
func (p environProvider) correctLocalhostURLs(cfg *config.Config, providerCfg *environConfig) (*config.Config, error) {
	attrs := cfg.AllAttrs()
	updatedAttrs := make(map[string]interface{})
	for _, key := range config.ProxyAttributes {
		anAttr := attrs[key]
		if anAttr == nil {
			continue
		}
		var attrStr string
		var isString bool
		if attrStr, isString = anAttr.(string); !isString || attrStr == "" {
			continue
		}
		newValue, err := p.swapLocalhostForBridgeIP(attrStr, providerCfg)
		if err != nil {
			return nil, errors.Trace(err)
		}
		updatedAttrs[key] = newValue
		logger.Infof("\nAttribute %q is set to (%v)\n", key, newValue)
	}
	// Update desired attributes on current configuration
	return cfg.Apply(updatedAttrs)
}

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (p environProvider) RestrictedConfigAttributes() []string {
	return []string{ContainerKey, NetworkBridgeKey, RootDirKey, "proxy-ssh"}
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (p environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return cfg, nil
}

// PrepareForBootstrap implements environs.EnvironProvider.PrepareForBootstrap.
func (p environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	attrs := map[string]interface{}{
		// We must not proxy SSH through the API server in a
		// local provider environment. Besides not being useful,
		// it may not work; there is no requirement for sshd to
		// be available on machine-0.
		"proxy-ssh": false,
	}
	if _, ok := cfg.AgentVersion(); !ok {
		attrs["agent-version"] = version.Current.Number.String()
	}
	if namespace, _ := cfg.UnknownAttrs()["namespace"].(string); namespace == "" {
		username := os.Getenv("USER")
		if username == "" {
			u, err := userCurrent()
			if err != nil {
				return nil, errors.Annotate(err, "failed to determine username for namespace")
			}
			username = u.Username
		}
		attrs["namespace"] = fmt.Sprintf("%s-%s", username, cfg.Name())
	}

	setIfNotBlank := func(key, value string) {
		if value != "" {
			attrs[key] = value
		}
	}
	// If the user has specified no values for any of the four normal
	// proxies, then look in the environment and set them.
	logger.Tracef("Look for proxies?")
	if cfg.HttpProxy() == "" &&
		cfg.HttpsProxy() == "" &&
		cfg.FtpProxy() == "" &&
		cfg.NoProxy() == "" {
		proxySettings := proxy.DetectProxies()
		logger.Tracef("Proxies detected %#v", proxySettings)
		setIfNotBlank(config.HttpProxyKey, proxySettings.Http)
		setIfNotBlank(config.HttpsProxyKey, proxySettings.Https)
		setIfNotBlank(config.FtpProxyKey, proxySettings.Ftp)
		setIfNotBlank(config.NoProxyKey, proxySettings.NoProxy)
	}
	if version.Current.OS == version.Ubuntu {
		if cfg.AptHttpProxy() == "" &&
			cfg.AptHttpsProxy() == "" &&
			cfg.AptFtpProxy() == "" {
			proxySettings, err := detectPackageProxies()
			if err != nil {
				return nil, errors.Trace(err)
			}
			setIfNotBlank(config.AptHttpProxyKey, proxySettings.Http)
			setIfNotBlank(config.AptHttpsProxyKey, proxySettings.Https)
			setIfNotBlank(config.AptFtpProxyKey, proxySettings.Ftp)
		}
	}

	cfg, err := cfg.Apply(attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Make sure everything is valid.
	cfg, err = p.Validate(cfg, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// The user must not set bootstrap-ip; this is determined by the provider,
	// and its presence used to determine whether the environment has yet been
	// bootstrapped.
	if _, ok := cfg.UnknownAttrs()["bootstrap-ip"]; ok {
		return nil, errors.Errorf("bootstrap-ip must not be specified")
	}
	err = checkLocalPort(cfg.StatePort(), "state port")
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = checkLocalPort(cfg.APIPort(), "API port")
	if err != nil {
		return nil, errors.Trace(err)
	}

	return p.Open(cfg)
}

// swapLocalhostForBridgeIP substitutes bridge ip for localhost. Non-localhost values are not modified.
func (p environProvider) swapLocalhostForBridgeIP(originalURL string, providerConfig *environConfig) (string, error) {
	// TODO(anastasia) 2014-10-31 Bug#1385277 Parse method does not cater for malformed URL, eg. localhost:8080
	parsedUrl, err := url.Parse(originalURL)
	if err != nil {
		return "", errors.Trace(err)
	}

	isLoopback, _, port := isLoopback(parsedUrl.Host)
	if !isLoopback {
		// If not loopback host address, return current attribute value
		return originalURL, nil
	}
	//If localhost is specified, use its network bridge ip
	bridgeAddress, nwerr := getAddressForInterface(providerConfig.networkBridge())
	if nwerr != nil {
		return "", errors.Trace(nwerr)
	}
	parsedUrl.Host = bridgeAddress + port
	return parsedUrl.String(), nil
}

// isLoopback returns whether given url is a loopback url.
// The argument to the method is expected to be in the form of
// host:port where host and port are also returned as distinct values.
func isLoopback(hostAndPort string) (isLoopback bool, host, port string) {
	host, port = getHostAndPort(hostAndPort)
	isLoopback = strings.ToLower(host) == "localhost"
	if !isLoopback {
		ip := net.ParseIP(host)
		isLoopback = ip != nil && ip.IsLoopback()
	}
	return
}

// getHostAndPort expects argument in the form host:port and
// returns host and port as distinctive strings.
func getHostAndPort(original string) (host, port string) {
	// Host and post specification is host:port
	hostAndPortRegexp := regexp.MustCompile(`(?P<host>\[?[::]*[^:]+)(?P<port>$|:[^:]+$)`)

	matched := hostAndPortRegexp.FindStringSubmatch(original)
	if len(matched) == 0 {
		// Passed in parameter is not in the form host:port.
		// Let's not mess with it.
		return original, ""
	}

	// For the string in the form host:port, FindStringSubmatch above
	// will return {host:port, host, :port}
	host = matched[1]
	port = matched[2]

	// For hosts like [::1], remove brackets
	if strings.Contains(host, "[") {
		host = host[1 : len(host)-1]
	}

	// For hosts like ::1, substring :1 is not a port!
	if strings.Contains(host, port) {
		port = ""
	}
	return
}

// checkLocalPort checks that the passed port is not used so far.
var checkLocalPort = func(port int, description string) error {
	logger.Infof("checking %s", description)
	// Try to connect the port on localhost.
	address := net.JoinHostPort("localhost", strconv.Itoa(port))
	// TODO(mue) Add a timeout?
	conn, err := net.Dial("tcp", address)
	if err != nil {
		if isConnectionRefused(err) {
			// we're expecting to get conn refused
			return nil
		}
		// some other error happened
		return errors.Trace(err)
	}
	// Connected, so port is in use.
	err = conn.Close()
	if err != nil {
		return err
	}
	return errors.Errorf("cannot use %d as %s, already in use", port, description)
}

// isConnectionRefused indicates if the err was caused by a refused connection.
func isConnectionRefused(err error) bool {
	if err, ok := err.(*net.OpError); ok {
		// go 1.4 and earlier
		if err.Err == syscall.ECONNREFUSED {
			return true
		}
		// go 1.5 and later
		if err, ok := err.Err.(*os.SyscallError); ok {
			return err.Err == syscall.ECONNREFUSED
		}
	}
	return false
}

// Validate implements environs.EnvironProvider.Validate.
func (provider environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to validate unknown attrs")
	}
	localConfig := newEnvironConfig(cfg, validated)
	// Set correct default network bridge if needed
	// fix for http://pad.lv/1394450
	localConfig.setDefaultNetworkBridge()
	// Before potentially creating directories, make sure that the
	// root directory has not changed.
	if localConfig.namespace() == "" {
		return nil, errors.New("missing namespace, config not prepared")
	}
	containerType := localConfig.container()
	if old != nil {
		oldLocalConfig, err := provider.newConfig(old)
		if err != nil {
			return nil, errors.Annotatef(err, "old config is not a valid local config: %v", old)
		}
		if containerType != oldLocalConfig.container() {
			return nil, errors.Errorf("cannot change container from %q to %q",
				oldLocalConfig.container(), containerType)
		}
		if localConfig.rootDir() != oldLocalConfig.rootDir() {
			return nil, errors.Errorf("cannot change root-dir from %q to %q",
				oldLocalConfig.rootDir(),
				localConfig.rootDir())
		}
		if localConfig.networkBridge() != oldLocalConfig.networkBridge() {
			return nil, errors.Errorf("cannot change network-bridge from %q to %q",
				oldLocalConfig.rootDir(),
				localConfig.rootDir())
		}
		if localConfig.storagePort() != oldLocalConfig.storagePort() {
			return nil, errors.Errorf("cannot change storage-port from %v to %v",
				oldLocalConfig.storagePort(),
				localConfig.storagePort())
		}
		if localConfig.namespace() != oldLocalConfig.namespace() {
			return nil, errors.Errorf("cannot change namespace from %v to %v",
				oldLocalConfig.namespace(),
				localConfig.namespace())
		}
	}
	// Currently only supported containers are "lxc" and "kvm".
	if containerType != instance.LXC && containerType != instance.KVM {
		return nil, errors.Errorf("unsupported container type: %q", containerType)
	}
	dir, err := utils.NormalizePath(localConfig.rootDir())
	if err != nil {
		return nil, err
	}
	if dir == "." {
		dir = osenv.JujuHomePath(cfg.Name())
	}
	// Always assign the normalized path.
	localConfig.attrs["root-dir"] = dir

	// If the user hasn't already specified a value, set it to the
	// given value.
	defineIfNot := func(keyName string, value interface{}) {
		if _, defined := cfg.AllAttrs()[keyName]; !defined {
			logger.Infof("lxc-clone is enabled. Switching %s to %v", keyName, value)
			localConfig.attrs[keyName] = value
		}
	}

	// If we're cloning, and the user hasn't specified otherwise,
	// prefer to skip update logic.
	if useClone, _ := localConfig.LXCUseClone(); useClone && containerType == instance.LXC {
		defineIfNot("enable-os-refresh-update", true)
		defineIfNot("enable-os-upgrade", false)
	}

	// Apply the coerced unknown values back into the config.
	return cfg.Apply(localConfig.attrs)
}

// BoilerplateConfig implements environs.EnvironProvider.BoilerplateConfig.
func (environProvider) BoilerplateConfig() string {
	return `
# https://juju.ubuntu.com/docs/config-local.html
local:
    type: local

    # root-dir holds the directory that is used for the storage files and
    # database. The default location is $JUJU_HOME/<env-name>.
    # $JUJU_HOME defaults to ~/.juju. Override if needed.
    #
    # root-dir: ~/.juju/local

    # storage-port holds the port where the local provider starts the
    # HTTP file server. Override the value if you have multiple local
    # providers, or if the default port is used by another program.
    #
    # storage-port: 8040

    # network-bridge holds the name of the LXC network bridge to use.
    # Override if the default LXC network bridge is different.
    #
    #
    # network-bridge: lxcbr0

    # The default series to deploy the state-server and charms on.
    # Make sure to uncomment the following option and set the value to
    # precise or trusty as desired.
    #
    # default-series: trusty

    # Whether or not to refresh the list of available updates for an
    # OS. The default option of true is recommended for use in
    # production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-refresh-update: true

    # Whether or not to perform OS upgrades when machines are
    # provisioned. The default option of true is recommended for use
    # in production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-upgrade: true

`[1:]
}

// SecretAttrs implements environs.EnvironProvider.SecretAttrs.
func (environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	// don't have any secret attrs
	return nil, nil
}

func (p environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return newEnvironConfig(valid, valid.UnknownAttrs()), nil
}
