// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/loggo"
)

// Logger for the Azure provider.
var logger = loggo.GetLogger("juju.environs.azure")

type azureEnvironProvider struct{}

// azureEnvironProvider implements EnvironProvider.
var _ environs.EnvironProvider = (*azureEnvironProvider)(nil)

// Open is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Debugf("opening environment %q.", cfg.Name())
	return NewEnviron(cfg)
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
