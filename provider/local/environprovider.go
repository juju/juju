// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
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
	if err := VerifyPrerequisites(); err != nil {
		logger.Errorf("failed verification of local provider prerequisites: %v", err)
		return nil, err
	}
	environ := &localEnviron{name: cfg.Name()}
	if err := environ.SetConfig(cfg); err != nil {
		logger.Errorf("failure setting config: %v", err)
		return nil, err
	}
	return environ, nil
}

// Prepare implements environs.EnvironProvider.Prepare.
func (p environProvider) Prepare(cfg *config.Config) (environs.Environ, error) {
	// TODO prepare environment
	return p.Open(cfg)
}

// Validate implements environs.EnvironProvider.Validate.
func (provider environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		logger.Errorf("failed to validate unknown attrs: %v", err)
		return nil, err
	}
	localConfig := newEnvironConfig(cfg, validated)
	// Before potentially creating directories, make sure that the
	// root directory has not changed.
	if old != nil {
		oldLocalConfig, err := provider.newConfig(old)
		if err != nil {
			return nil, fmt.Errorf("old config is not a valid local config: %v", old)
		}
		if localConfig.rootDir() != oldLocalConfig.rootDir() {
			return nil, fmt.Errorf("cannot change root-dir from %q to %q",
				oldLocalConfig.rootDir(),
				localConfig.rootDir())
		}
		if localConfig.storagePort() != oldLocalConfig.storagePort() {
			return nil, fmt.Errorf("cannot change storage-port from %v to %v",
				oldLocalConfig.storagePort(),
				localConfig.storagePort())
		}
		if localConfig.sharedStoragePort() != oldLocalConfig.sharedStoragePort() {
			return nil, fmt.Errorf("cannot change shared-storage-port from %v to %v",
				oldLocalConfig.sharedStoragePort(),
				localConfig.sharedStoragePort())
		}
	}
	dir := utils.NormalizePath(localConfig.rootDir())
	if dir == "." {
		dir = config.JujuHomePath(cfg.Name())
	}
	// Always assign the normalized path.
	localConfig.attrs["root-dir"] = dir

	// Apply the coerced unknown values back into the config.
	return cfg.Apply(localConfig.attrs)
}

// BoilerplateConfig implements environs.EnvironProvider.BoilerplateConfig.
func (environProvider) BoilerplateConfig() string {
	return `
## https://juju.ubuntu.com/docs/config-local.html
local:
  type: local
  admin-secret: {{rand}}
  # Override the directory that is used for the storage files and database.
  # The default location is $JUJU_HOME/<ENV>.
  # $JUJU_HOME defaults to ~/.juju
  # root-dir: ~/.juju/local
  # Override the storage port if you have multiple local providers, or if the
  # default port is used by another program.
  # storage-port: 8040
  # Override the shared storage port if you have multiple local providers, or if the
  # default port is used by another program.
  # shared-storage-port: 8041

`[1:]
}

// SecretAttrs implements environs.EnvironProvider.SecretAttrs.
func (environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	// don't have any secret attrs
	return nil, nil
}

// Location specific methods that are able to be called by any instance that
// has been created by this provider type.  So a machine agent may well call
// these methods to find out its own address or instance id.

// PublicAddress implements environs.EnvironProvider.PublicAddress.
func (environProvider) PublicAddress() (string, error) {
	// Get the IPv4 address from eth0
	return getAddressForInterface("eth0")
}

// PrivateAddress implements environs.EnvironProvider.PrivateAddress.
func (environProvider) PrivateAddress() (string, error) {
	// Get the IPv4 address from eth0
	return getAddressForInterface("eth0")
}

func (p environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return newEnvironConfig(valid, valid.UnknownAttrs()), nil
}
