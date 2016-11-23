// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/openstack"
)

const (
	providerType = "rackspace"
)

func init() {
	osProvider := &openstack.EnvironProvider{
		Credentials{},
		&rackspaceConfigurator{},
		&firewallerFactory{},
		openstack.FlavorFilterFunc(acceptRackspaceFlavor),
		rackspaceNetworkingDecorator{},
	}
	providerInstance = &environProvider{
		osProvider,
	}
	environs.RegisterProvider(providerType, providerInstance)
}
