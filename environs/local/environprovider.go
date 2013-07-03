// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"os"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.environs.local")

var _ environs.EnvironProvider = (*environProvider)(nil)

type environProvider struct{}

var provider environProvider

func init() {
	environs.RegisterProvider("local", &environProvider{})
}

var (
	defaultPublicStorageDir  = "/var/lib/juju/local/%s/public"
	defaultPrivateStorageDir = "/var/lib/juju/local/%s/private"
)

// Open implements environs.EnvironProvider.Open.
func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Infof("opening environment %q", cfg.Name())
	environ := &localEnviron{name: cfg.Name()}
	err := environ.SetConfig(cfg)
	if err != nil {
		logger.Errorf("failure setting config: %v", err)
		return nil, err
	}
	return environ, nil
}

// Validate implements environs.EnvironProvider.Validate.
func (provider environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	v, err := configChecker.Coerce(cfg.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	localConfig := newEnvironConfig(cfg, v.(map[string]interface{}))
	// Before potentially creating directories, make sure that the
	// public storage and private storage values have not changed.
	if old != nil {
		oldLocalConfig, err := provider.newConfig(old)
		if err != nil {
			return nil, fmt.Errorf("old config is not a valid local config: %v", old)
		}
		if localConfig.publicStorageDir() != oldLocalConfig.publicStorageDir() {
			return nil, fmt.Errorf("cannot change shared-storage from %q to %q",
				oldLocalConfig.publicStorageDir(),
				localConfig.publicStorageDir())
		}
		if localConfig.privateStorageDir() != oldLocalConfig.privateStorageDir() {
			return nil, fmt.Errorf("cannot change storage from %q to %q",
				oldLocalConfig.privateStorageDir(),
				localConfig.privateStorageDir())
		}
	}
	dir := utils.NormalizePath(localConfig.publicStorageDir())
	if dir == "." {
		dir = fmt.Sprintf(defaultPublicStorageDir, localConfig.namespace())
		localConfig.attrs["shared-storage"] = dir
	}
	logger.Tracef("ensure shared-storage dir %s exists", dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("failed to make directory for shared storage at %s: %v", dir, err)
		return nil, err
	}

	dir = utils.NormalizePath(localConfig.privateStorageDir())
	if dir == "." {
		dir = fmt.Sprintf(defaultPrivateStorageDir, localConfig.namespace())
		localConfig.attrs["storage"] = dir
	}
	logger.Tracef("ensure storage dir %s exists", dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("failed to make directory for storage at %s: %v", dir, err)
		return nil, err
	}

	// Apply the coerced unknown values back into the config.
	return cfg.Apply(localConfig.attrs)
}

// BoilerplateConfig implements environs.EnvironProvider.BoilerplateConfig.
func (environProvider) BoilerplateConfig() string {
	return `
## https://juju.ubuntu.com/get-started/local/
local:
  type: local
  # Override the storage location to store the private files for this environment in the
  # specified location.  The default location is /var/lib/juju/local/<USER>-<ENV>/private
  # storage: ~/.juju/local/private
  # Override the shared-storage location to store the public files for this environment in the
  # specified location.  The default location is /var/lib/juju/local/<USER>-<ENV>/public
  # shared-storage: ~/.juju/local/public

`[1:]
}

// SecretAttrs implements environs.EnvironProvider.SecretAttrs.
func (environProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	// don't have any secret attrs
	return nil, nil
}

// Location specific methods that are able to be called by any instance that
// has been created by this provider type.  So a machine agent may well call
// these methods to find out its own address or instance id.

// PublicAddress implements environs.EnvironProvider.PublicAddress.
func (environProvider) PublicAddress() (string, error) {
	return "", fmt.Errorf("not implemented")
}

// PrivateAddress implements environs.EnvironProvider.PrivateAddress.
func (environProvider) PrivateAddress() (string, error) {
	return "", fmt.Errorf("not implemented")
}

// InstanceId implements environs.EnvironProvider.InstanceId.
func (environProvider) InstanceId() (instance.Id, error) {
	return "", fmt.Errorf("not implemented")
}

func (environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := provider.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return newEnvironConfig(valid, valid.UnknownAttrs()), nil
}
