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
	osProvider := openstack.EnvironProvider{
		openstack.OpenstackCredentials{},
		&rackspaceConfigurator{},
		&firewallerFactory{},
	}
	providerInstance = &environProvider{
		osProvider,
	}
	environs.RegisterProvider(providerType, providerInstance)
}
