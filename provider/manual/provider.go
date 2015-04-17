// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
)

type manualProvider struct{}

// Verify that we conform to the interface.
var _ environs.EnvironProvider = (*manualProvider)(nil)

var errNoBootstrapHost = errors.New("bootstrap-host must be specified")

var initUbuntuUser = manual.InitUbuntuUser

func ensureBootstrapUbuntuUser(ctx environs.BootstrapContext, cfg *environConfig) error {
	err := initUbuntuUser(cfg.bootstrapHost(), cfg.bootstrapUser(), cfg.AuthorizedKeys(), ctx.GetStdin(), ctx.GetStdout())
	if err != nil {
		logger.Errorf("initializing ubuntu user: %v", err)
		return err
	}
	logger.Infof("initialized ubuntu user")
	return nil
}

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (p manualProvider) RestrictedConfigAttributes() []string {
	return []string{"bootstrap-host", "bootstrap-user"}
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (p manualProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	// Not even sure if this will ever make sense.
	return nil, errors.NotImplementedf("PrepareForCreateEnvironment")
}

func (p manualProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	if _, ok := cfg.UnknownAttrs()["storage-auth-key"]; !ok {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, err
		}
		cfg, err = cfg.Apply(map[string]interface{}{
			"storage-auth-key": uuid.String(),
		})
		if err != nil {
			return nil, err
		}
	}
	if use, ok := cfg.UnknownAttrs()["use-sshstorage"].(bool); ok && !use {
		return nil, fmt.Errorf("use-sshstorage must not be specified")
	}
	envConfig, err := p.validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	cfg, err = cfg.Apply(envConfig.attrs)
	if err != nil {
		return nil, err
	}
	envConfig = newEnvironConfig(cfg, envConfig.attrs)
	if err := ensureBootstrapUbuntuUser(ctx, envConfig); err != nil {
		return nil, err
	}
	return p.open(envConfig)
}

func (p manualProvider) Open(cfg *config.Config) (environs.Environ, error) {
	_, err := p.validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	// validate adds missing manual-specific config attributes
	// with their defaults in the result; we don't wnat that in
	// Open.
	envConfig := newEnvironConfig(cfg, cfg.UnknownAttrs())
	return p.open(envConfig)
}

func (p manualProvider) open(cfg *environConfig) (environs.Environ, error) {
	env := &manualEnviron{cfg: cfg}
	// Need to call SetConfig to initialise storage.
	if err := env.SetConfig(cfg.Config); err != nil {
		return nil, err
	}
	return env, nil
}

func checkImmutableString(cfg, old *environConfig, key string) error {
	if old.attrs[key] != cfg.attrs[key] {
		return fmt.Errorf("cannot change %s from %q to %q", key, old.attrs[key], cfg.attrs[key])
	}
	return nil
}

func (p manualProvider) validate(cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	envConfig := newEnvironConfig(cfg, validated)
	if envConfig.bootstrapHost() == "" {
		return nil, errNoBootstrapHost
	}
	// Check various immutable attributes.
	if old != nil {
		oldEnvConfig, err := p.validate(old, nil)
		if err != nil {
			return nil, err
		}
		for _, key := range [...]string{
			"bootstrap-user",
			"bootstrap-host",
			"storage-listen-ip",
		} {
			if err = checkImmutableString(envConfig, oldEnvConfig, key); err != nil {
				return nil, err
			}
		}
		oldPort, newPort := oldEnvConfig.storagePort(), envConfig.storagePort()
		if oldPort != newPort {
			return nil, fmt.Errorf("cannot change storage-port from %q to %q", oldPort, newPort)
		}
		oldUseSSHStorage, newUseSSHStorage := oldEnvConfig.useSSHStorage(), envConfig.useSSHStorage()
		if oldUseSSHStorage != newUseSSHStorage && newUseSSHStorage == true {
			return nil, fmt.Errorf("cannot change use-sshstorage from %v to %v", oldUseSSHStorage, newUseSSHStorage)
		}
	}

	// If the user hasn't already specified a value, set it to the
	// given value.
	defineIfNot := func(keyName string, value interface{}) {
		if _, defined := cfg.AllAttrs()[keyName]; !defined {
			logger.Infof("%s was not defined. Defaulting to %v.", keyName, value)
			envConfig.attrs[keyName] = value
		}
	}

	// If the user hasn't specified a value, refresh the
	// available updates, but don't upgrade.
	defineIfNot("enable-os-refresh-update", true)
	defineIfNot("enable-os-upgrade", false)

	return envConfig, nil
}

func (p manualProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	envConfig, err := p.validate(cfg, old)
	if err != nil {
		return nil, err
	}
	return cfg.Apply(envConfig.attrs)
}

func (_ manualProvider) BoilerplateConfig() string {
	return `
manual:
    type: manual
    # bootstrap-host holds the host name of the machine where the
    # bootstrap machine agent will be started.
    bootstrap-host: somehost.example.com

    # bootstrap-user specifies the user to authenticate as when
    # connecting to the bootstrap machine. It defaults to
    # the current user.
    # bootstrap-user: joebloggs

    # storage-listen-ip specifies the IP address that the
    # bootstrap machine's Juju storage server will listen
    # on. By default, storage will be served on all
    # network interfaces.
    # storage-listen-ip:

    # storage-port specifes the TCP port that the
    # bootstrap machine's Juju storage server will listen
    # on. It defaults to ` + fmt.Sprint(defaultStoragePort) + `
    # storage-port: ` + fmt.Sprint(defaultStoragePort) + `

    # Whether or not to refresh the list of available updates for an
    # OS. The default option of true is recommended for use in
    # production systems.
    #
    # enable-os-refresh-update: true

    # Whether or not to perform OS upgrades when machines are
    # provisioned. The default option of false is set so that Juju
    # does not subsume any other way the system might be
    # maintained.
    #
    # enable-os-upgrade: false

`[1:]
}

func (p manualProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	envConfig, err := p.validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	attrs := make(map[string]string)
	attrs["storage-auth-key"] = envConfig.storageAuthKey()
	return attrs, nil
}
