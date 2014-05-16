// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"syscall"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
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
				return nil, fmt.Errorf("failed to determine username for namespace: %v", err)
			}
			username = u.Username
		}
		var err error
		namespace = fmt.Sprintf("%s-%s", username, cfg.Name())
		cfg, err = cfg.Apply(map[string]interface{}{"namespace": namespace})
		if err != nil {
			return nil, fmt.Errorf("failed to create namespace: %v", err)
		}
	}
	// Do the initial validation on the config.
	localConfig, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	if err := VerifyPrerequisites(localConfig.container()); err != nil {
		return nil, fmt.Errorf("failed verification of local provider prerequisites: %v", err)
	}
	environ := &localEnviron{name: cfg.Name()}
	if err := environ.SetConfig(cfg); err != nil {
		return nil, fmt.Errorf("failure setting config: %v", err)
	}
	return environ, nil
}

var detectAptProxies = utils.DetectAptProxies

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
		proxy := osenv.DetectProxies()
		logger.Tracef("Proxies detected %#v", proxy)
		setIfNotBlank("http-proxy", proxy.Http)
		setIfNotBlank("https-proxy", proxy.Https)
		setIfNotBlank("ftp-proxy", proxy.Ftp)
		setIfNotBlank("no-proxy", proxy.NoProxy)
	}
	if cfg.AptHttpProxy() == "" &&
		cfg.AptHttpsProxy() == "" &&
		cfg.AptFtpProxy() == "" {
		proxy, err := detectAptProxies()
		if err != nil {
			return nil, err
		}
		setIfNotBlank("apt-http-proxy", proxy.Http)
		setIfNotBlank("apt-https-proxy", proxy.Https)
		setIfNotBlank("apt-ftp-proxy", proxy.Ftp)
	}
	if len(attrs) > 0 {
		cfg, err = cfg.Apply(attrs)
		if err != nil {
			return nil, err
		}
	}

	return p.Open(cfg)
}

// checkLocalPort checks that the passed port is not used so far.
var checkLocalPort = func(port int, description string) error {
	logger.Infof("checking %s", description)
	// Try to connect the port on localhost.
	address := fmt.Sprintf("localhost:%d", port)
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
		return nil, fmt.Errorf("failed to validate unknown attrs: %v", err)
	}
	localConfig := newEnvironConfig(cfg, validated)
	// Before potentially creating directories, make sure that the
	// root directory has not changed.
	containerType := localConfig.container()
	if old != nil {
		oldLocalConfig, err := provider.newConfig(old)
		if err != nil {
			return nil, fmt.Errorf("old config is not a valid local config: %v", old)
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
    # default-series: precise

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
