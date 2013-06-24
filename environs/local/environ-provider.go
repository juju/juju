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

func init() {
	environs.RegisterProvider("local", &environProvider{})
}

const (
	defaultPublicStorageDir  = "/var/lib/juju/local/%s/public"
	defaultPrivateStorageDir = "/var/lib/juju/local/%s/public"
)

// Open implements environs.EnvironProvider.Open.
func (*environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Infof("opening environment %q", cfg.Name())
	environ := &localEnviron{}
	err := environ.SetConfig(cfg)
	if err != nil {
		logger.Errorf("failure setting config: %v", err)
		return nil, err
	}
	return environ, nil
}

func ensureDirExists(path string) error {
	// If the directory doesn't exist, try to make it.
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
func (*environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	v, err := configChecker.Coerce(cfg.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	localConfig := &environConfig{cfg, v.(map[string]interface{})}
	if localConfig.publicStorageDir() == "" {
		dir := fmt.Sprintf(defaultPublicStorageDir, cfg.Name())
		if err := ensureDirExists(dir); err != nil {
			return nil, err
		}
		localConfig.attrs["public-storate"] = dir
	}

	if localConfig.privateStorageDir() == "" {
		dir := fmt.Sprintf(defaultPrivateStorageDir, cfg.Name())
		if err := ensureDirExists(dir); err != nil {
			return nil, err
		}
		localConfig.attrs["private-storate"] = dir
	}

	// Apply the coerced unknown values back into the config.
	return cfg.Apply(localConfig.attrs)
}

// BoilerplateConfig implements environs.EnvironProvider.BoilerplateConfig.
func (*environProvider) BoilerplateConfig() string {
	panic("unimplemented")
}

// SecretAttrs implements environs.EnvironProvider.SecretAttrs.
func (*environProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	// don't have any secret attrs
	return nil, nil
}

// PublicAddress implements environs.EnvironProvider.PublicAddress.
func (*environProvider) PublicAddress() (string, error) {
	panic("unimplemented")
}

// PrivateAddress implements environs.EnvironProvider.PrivateAddress.
func (*environProvider) PrivateAddress() (string, error) {
	panic("unimplemented")
}

// InstanceId implements environs.EnvironProvider.InstanceId.
func (*environProvider) InstanceId() (instance.Id, error) {
	panic("unimplemented")
}

func (p *environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}
