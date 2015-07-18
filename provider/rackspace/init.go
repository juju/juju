// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/juju/environs"
<<<<<<< HEAD
	"github.com/juju/juju/provider/openstack"
=======
>>>>>>> modifications to opestack provider applied
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType = "rackspace"
)

func init() {
	openstackProvider, err := environs.Provider("openstack")
	if err != nil {
		logger.Errorf("Can't find openstack provider, error: %s", err)
		return
	}
	providerInstance = environProvider{
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
}
