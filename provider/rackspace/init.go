// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/juju/environs"
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
		openstackProvider: openstackProvider,
	}
	environs.RegisterProvider(providerType, providerInstance)

	registry.RegisterEnvironStorageProviders(providerType)
}
