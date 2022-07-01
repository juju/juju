// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/go-goose/goose/v5/client"
	"github.com/go-goose/goose/v5/identity"

	"github.com/juju/juju/v2/environs"
	"github.com/juju/juju/v2/provider/openstack"
)

const (
	providerType = "rackspace"
)

func init() {
	osProvider := &openstack.EnvironProvider{
		ProviderCredentials: Credentials{},
		Configurator:        &rackspaceConfigurator{},
		FirewallerFactory:   &firewallerFactory{},
		FlavorFilter:        openstack.FlavorFilterFunc(acceptRackspaceFlavor),
		NetworkingDecorator: rackspaceNetworkingDecorator{},
		ClientFromEndpoint: func(endpoint string) client.AuthenticatingClient {
			return client.NewClient(&identity.Credentials{URL: endpoint}, 0, nil)
		},
	}
	providerInstance = &environProvider{
		osProvider,
	}
	environs.RegisterProvider(providerType, providerInstance)
}
