// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"

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

// Open implements environs.EnvironProvider.Open.
func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	return nil, fmt.Errorf("not implemented")
}

// Validate implements environs.EnvironProvider.Validate.
func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	return nil, fmt.Errorf("not implemented")
}

// BoilerplateConfig implements environs.EnvironProvider.BoilerplateConfig.
func (environProvider) BoilerplateConfig() string {
	return "not implemented"
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
