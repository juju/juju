// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
)

var _ environs.EnvironProvider = (*environProvider)(nil)

type environProvider struct{}

func init() {
	environs.RegisterProvider("local", &environProvider{})
}

// Open implements environs.EnvironProvider.Open.
func (*environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	panic("unimplemented")
}

// Validate implements environs.EnvironProvider.Validate.
func (*environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	panic("unimplemented")
}

// BoilerplateConfig implements environs.EnvironProvider.BoilerplateConfig.
func (*environProvider) BoilerplateConfig() string {
	panic("unimplemented")
}

// SecretAttrs implements environs.EnvironProvider.SecretAttrs.
func (*environProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	panic("unimplemented")
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
