// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType = "rackspace"
)

func init() {
	provider, err := environs.Provider("openstack")
	if err != nil {
		logger.Errorf("Can't find openstack provider, error: %s", err)
		return
	}
	if osProvider, ok := provider.(*openstack.EnvironProvider); ok {
		osProvider.Configurator = &rackspaceConfigurator{}
		providerInstance = environProvider{
			osProvider,
		}
		environs.RegisterProvider(providerType, providerInstance)

		registry.RegisterEnvironStorageProviders(providerType, openstack.CinderProviderType)
	} else {
		logger.Errorf("Openstack provider has wrong type.")
		return
	}
}
