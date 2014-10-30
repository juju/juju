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
	"syscall"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/apt"
	"github.com/juju/utils/proxy"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.provider.local")

var _ environs.EnvironProvider = (*environProvider)(nil)

type environProvider struct{}

var providerInstance = &environProvider{}

func init() {
	environs.RegisterProvider(provider.Local, providerInstance)
}

var userCurrent = user.Current

// Open implements environs.EnvironProvider.Open.
func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Infof("opening environment %q", cfg.Name())
	if _, ok := cfg.AgentVersion(); !ok {
		newCfg, err := cfg.Apply(map[string]interface{}{
			"agent-version": version.Current.Number.String(),
		})
		if err != nil {
			return nil, err
		}
		cfg = newCfg
	}
	// Set the "namespace" attribute. We do this here, and not in Prepare,
	// for backwards compatibility: older versions did not store the namespace
	// in config.
	if namespace, _ := cfg.UnknownAttrs()["namespace"].(string); namespace == "" {
		username := os.Getenv("USER")
		if username == "" {
			u, err := userCurrent()
			if err != nil {
				return nil, errors.Annotate(err, "failed to determine username for namespace")
			}
			username = u.Username
		}
		var err error
		namespace = fmt.Sprintf("%s-%s", username, cfg.Name())
		cfg, err = cfg.Apply(map[string]interface{}{"namespace": namespace})
		if err != nil {
			return nil, errors.Annotate(err, "failed to create namespace")
		}
	}
	// Do the initial validation on the config.
	localConfig, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	if err := VerifyPrerequisites(localConfig.container()); err != nil {
		return nil, errors.Annotate(err, "failed verification of local provider prerequisites")
	}
	if cfg, err = providerInstance.correctLocalhostURLs(cfg, localConfig); err != nil {
		return nil, errors.Annotate(err, "failed to translate proxy config settings")
	}
	environ := &localEnviron{name: cfg.Name()}
	if err := environ.SetConfig(cfg); err != nil {
		return nil, errors.Annotate(err, "failure setting config")
	}
	return environ, nil
}

// correctLocalhostURLs exams proxy attributes and changes URL values pointing to localhost to use bridge IP.
func (p environProvider) correctLocalhostURLs(cfg *config.Config, providerCfg *environConfig) (*config.Config, error) {
	// Attribute names that could contain references to localhost in their values
	proxyAttrs := []string{
		"http-proxy",
		"https-proxy",
		"ftp-proxy",
		"apt-http-proxy",
		"apt-https-proxy",
		"apt-ftp-proxy",
	}
	// Current configuration attributes
	attrs := cfg.AllAttrs()

	newAttrs := make(map[string]interface{})
	for _, key := range proxyAttrs {
		anAttr := attrs[key]
		if anAttr == nil {
			continue
		}
		newValue, err := p.swapLocalhostForBridgeIP(anAttr.(string), providerCfg)
		if err != nil {
			return nil, err
		}
		newAttrs[key] = newValue
		logger.Infof("\nAttribute %q is set to (%v)\n", key, newValue)
	}
	// Update desired attributes on current configuration
	return cfg.Apply(newAttrs)
}

var detectAptProxies = apt.DetectProxies

// Prepare implements environs.EnvironProvider.Prepare.
func (p environProvider) Prepare(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	// The user must not set bootstrap-ip; this is determined by the provider,
	// and its presence used to determine whether the environment has yet been
	// bootstrapped.
	if _, ok := cfg.UnknownAttrs()["bootstrap-ip"]; ok {
		return nil, fmt.Errorf("bootstrap-ip must not be specified")
	}
	err := checkLocalPort(cfg.StatePort(), "state port")
	if err != nil {
		return nil, err
	}
	err = checkLocalPort(cfg.APIPort(), "API port")
	if err != nil {
		return nil, err
	}
	// If the user has specified no values for any of the three normal
	// proxies, then look in the environment and set them.
	attrs := map[string]interface{}{
		// We must not proxy SSH through the API server in a
		// local provider environment. Besides not being useful,
		// it may not work; there is no requirement for sshd to
		// be available on machine-0.
		"proxy-ssh": false,
	}
	setIfNotBlank := func(key, value string) {
		if value != "" {
			attrs[key] = value
		}
	}
	logger.Tracef("Look for proxies?")
	if cfg.HttpProxy() == "" &&
		cfg.HttpsProxy() == "" &&
		cfg.FtpProxy() == "" &&
		cfg.NoProxy() == "" {
		proxySettings := proxy.DetectProxies()
		logger.Tracef("Proxies detected %#v", proxySettings)
		setIfNotBlank("http-proxy", proxySettings.Http)
		setIfNotBlank("https-proxy", proxySettings.Https)
		setIfNotBlank("ftp-proxy", proxySettings.Ftp)
		setIfNotBlank("no-proxy", proxySettings.NoProxy)
	}
	if cfg.AptHttpProxy() == "" &&
		cfg.AptHttpsProxy() == "" &&
		cfg.AptFtpProxy() == "" {
		proxySettings, err := detectAptProxies()
		if err != nil {
			return nil, err
		}
		setIfNotBlank("apt-http-proxy", proxySettings.Http)
		setIfNotBlank("apt-https-proxy", proxySettings.Https)
		setIfNotBlank("apt-ftp-proxy", proxySettings.Ftp)
	}
	if len(attrs) > 0 {
		cfg, err = cfg.Apply(attrs)
		if err != nil {
			return nil, err
		}
	}

	return p.Open(cfg)
}

// swapLocalhostForBridgeIP substitutes bridge ip for localhost. Non-localhost values are not modified.
func (p environProvider) swapLocalhostForBridgeIP(u string, providerConfig *environConfig) (string, error) {
	if u == "" {
		return u, nil
	}
	// TODO {anastasia} Parse method does not cater for malformed URL, eg. localhost:8080
	parsedUrl, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	// Localhost url can be specified in 3 ways: localhost, 127.0.0.1 or ::1
	// This regular expression does not cater for a. subnets, eg. 127.0.0.1/8 nor b. digits preceding :: in ipv6 url, eg. 0::0:1.
	localHostRegexp := regexp.MustCompile("localhost|127\\.[\\d.]+|[0:]+1|\\[?::1]?")
	// Host and post specification is host:port
	hostPortRegexp := regexp.MustCompile("(?P<host>(\\[?[::]*[^:]+))(?P<port>($|:[^:]+$))")
	hostPort := parsedUrl.Host //parse url
	if !localHostRegexp.MatchString(hostPort) {
		// If not localhost, return current attribute value
		return u, nil
	}
	//If localhost is specified, use its network bridge ip
	bridgeAddress, nwerr := getAddressForInterface(providerConfig.networkBridge())
	if nwerr != nil {
		return "", nwerr
	}
	parsedUrl.Host = hostPortRegexp.ReplaceAllString(hostPort, fmt.Sprintf("%s$port", bridgeAddress))
	return parsedUrl.String(), nil
}

// checkLocalPort checks that the passed port is not used so far.
var checkLocalPort = func(port int, description string) error {
	logger.Infof("checking %s", description)
	// Try to connect the port on localhost.
	address := net.JoinHostPort("localhost", strconv.Itoa(port))
	// TODO(mue) Add a timeout?
	conn, err := net.Dial("tcp", address)
	if err != nil {
		if nerr, ok := err.(*net.OpError); ok {
			if nerr.Err == syscall.ECONNREFUSED {
				// No connection, so everything is fine.
				return nil
			}
		}
		return err
	}
	// Connected, so port is in use.
	err = conn.Close()
	if err != nil {
		return err
	}
	return fmt.Errorf("cannot use %d as %s, already in use", port, description)
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
	// Before potentially creating directories, make sure that the
	// root directory has not changed.
	containerType := localConfig.container()
	if old != nil {
		oldLocalConfig, err := provider.newConfig(old)
		if err != nil {
			return nil, errors.Annotatef(err, "old config is not a valid local config: %v", old)
		}
		if containerType != oldLocalConfig.container() {
			return nil, fmt.Errorf("cannot change container from %q to %q",
				oldLocalConfig.container(), containerType)
		}
		if localConfig.rootDir() != oldLocalConfig.rootDir() {
			return nil, fmt.Errorf("cannot change root-dir from %q to %q",
				oldLocalConfig.rootDir(),
				localConfig.rootDir())
		}
		if localConfig.networkBridge() != oldLocalConfig.networkBridge() {
			return nil, fmt.Errorf("cannot change network-bridge from %q to %q",
				oldLocalConfig.rootDir(),
				localConfig.rootDir())
		}
		if localConfig.storagePort() != oldLocalConfig.storagePort() {
			return nil, fmt.Errorf("cannot change storage-port from %v to %v",
				oldLocalConfig.storagePort(),
				localConfig.storagePort())
		}
	}
	// Currently only supported containers are "lxc" and "kvm".
	if containerType != instance.LXC && containerType != instance.KVM {
		return nil, fmt.Errorf("unsupported container type: %q", containerType)
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
		defineIfNot("enable-os-refresh-update", false)
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
