// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"gopkg.in/goose.v2/nova"

	"github.com/juju/juju/provider/openstack"
)

type rackspaceNetworkingDecorator struct{}

// DecorateNetworking is part of the openstack.NetworkingDecorator interface.
func (d rackspaceNetworkingDecorator) DecorateNetworking(n openstack.Networking) (openstack.Networking, error) {
	return rackspaceNetworking{n}, nil
}

type rackspaceNetworking struct {
	openstack.Networking
}

// DefaultNetworks is part of the openstack.Networking interface.
func (rackspaceNetworking) DefaultNetworks() ([]nova.ServerNetworks, error) {
	// These are the default rackspace networks, see:
	// http://docs.rackspace.com/servers/api/v2/cs-devguide/content/provision_server_with_networks.html
	return []nova.ServerNetworks{
		{NetworkId: "00000000-0000-0000-0000-000000000000"}, //Racksapce PublicNet
		{NetworkId: "11111111-1111-1111-1111-111111111111"}, //Rackspace ServiceNet
	}, nil
}
