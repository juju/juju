// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"os"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/loggo"
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

func ensureDirExists(path string) error {
	// If the directory doesn't exist, try to make it.
	logger.Tracef("ensure %q dir exists", path)
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		// Try to make the directory.
		if err = os.MkdirAll(path, 0755); err != nil {
			return err
		}
		return nil
	} else if err != nil {
		return err
	}
	if fileInfo.IsDir() {
		return nil
	}
	return fmt.Errorf("%q exists but is not a directory", path)
}

// Validate implements environs.EnvironProvider.Validate.
func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	v, err := configChecker.Coerce(cfg.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	localConfig := &environConfig{cfg, v.(map[string]interface{})}
	dir := localConfig.publicStorageDir()
	if dir == "" {
		dir = fmt.Sprintf(defaultPublicStorageDir, cfg.Name())
		localConfig.attrs["public-storage"] = dir
	}
	if err := ensureDirExists(dir); err != nil {
		return nil, err
	}

	dir = localConfig.privateStorageDir()
	if dir == "" {
		dir = fmt.Sprintf(defaultPrivateStorageDir, cfg.Name())
		localConfig.attrs["private-storage"] = dir
	}
	if err := ensureDirExists(dir); err != nil {
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
  # override if your workstation is running a different series to which you are deploying
  # default-series: precise

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
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}
