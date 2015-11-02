// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/juju/environs"
<<<<<<< HEAD
<<<<<<< HEAD
	"github.com/juju/juju/provider/openstack"
=======
>>>>>>> modifications to opestack provider applied
=======
	"github.com/juju/juju/provider/openstack"
>>>>>>> review comments implemented
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType = "rackspace"
)

func init() {
	osProvider := openstack.EnvironProvider{&rackspaceConfigurator{}, &firewallerFactory{}}
	providerInstance = environProvider{
		osProvider,
	}
<<<<<<< HEAD
<<<<<<< HEAD
	providerInstance = environProvider{
<<<<<<< HEAD
<<<<<<< HEAD
		openstackProvider,
	}
	environs.RegisterProvider(providerType, providerInstance)

	registry.RegisterEnvironStorageProviders(providerType, openstack.CinderProviderType)
=======
		openstackProvider: openstackProvider,
	}
	environs.RegisterProvider(providerType, providerInstance)

	registry.RegisterEnvironStorageProviders(providerType)
>>>>>>> modifications to opestack provider applied
=======
		openstackProvider,
	}
	environs.RegisterProvider(providerType, providerInstance)

	registry.RegisterEnvironStorageProviders(providerType, openstack.CinderProviderType)
>>>>>>> review comments implemented
=======
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
>>>>>>> Firewaller interface added, Waith ssh method reused
=======
	environs.RegisterProvider(providerType, providerInstance)

	registry.RegisterEnvironStorageProviders(providerType, openstack.CinderProviderType)
>>>>>>> review comments implemented
}
