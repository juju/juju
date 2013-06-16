// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
)

type azureEnvironProvider struct{}

// azureEnvironProvider implements EnvironProvider.
var _ environs.EnvironProvider = (*azureEnvironProvider)(nil)

// Open is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	panic("unimplemented")
}

// Validate is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	panic("unimplemented")
}

// BoilerplateConfig is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) BoilerplateConfig() string {
	panic("unimplemented")
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	panic("unimplemented")
}

// PublicAddress is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) PublicAddress() (string, error) {
	panic("unimplemented")
}

// PrivateAddress is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) PrivateAddress() (string, error) {
	panic("unimplemented")
}

// InstanceId is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) InstanceId() (instance.Id, error) {
	panic("unimplemented")
}
