// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/go-goose/goose/v5/neutron"
	"github.com/go-goose/goose/v5/nova"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
)

// Networking is an interface providing networking-related operations
// for an OpenStack Environ.
type Networking interface {
	// AllocatePublicIP allocates a public (floating) IP
	// to the specified instance.
	AllocatePublicIP(instance.Id) (*string, error)

	// ResolveNetworks takes either a network ID or label
	// with a string to specify whether the network is external
	// and returns the corresponding matching networks.
	ResolveNetworks(string, bool) ([]neutron.NetworkV2, error)

	// Subnets returns basic information about subnets known
	// by OpenStack for the environment.
	// Needed for Environ.Networking
	Subnets([]corenetwork.Id) ([]corenetwork.SubnetInfo, error)

	// CreatePort creates a port for a given network id with a subnet ID.
	CreatePort(string, string, corenetwork.Id) (*neutron.PortV2, error)

	// DeletePortByID attempts to remove a port using the given port ID.
	DeletePortByID(string) error

	// NetworkInterfaces requests information about the network
	// interfaces on the given list of instances.
	// Needed for Environ.Networking
	NetworkInterfaces(ids []instance.Id) ([]corenetwork.InterfaceInfos, error)

	// FindNetworks returns a set of internal or external network names
	// depending on the provided argument.
	FindNetworks(internal bool) (set.Strings, error)
}

// NetworkingBase describes the EnvironProvider methods needed for Networking.
type NetworkingBase interface {
	client() NetworkingAuthenticatingClient
	neutron() NetworkingNeutron
	nova() NetworkingNova
	ecfg() NetworkingEnvironConfig
}

// NetworkingNova describes the Nova methods needed for Networking.
type NetworkingNova interface {
	GetServer(string) (*nova.ServerDetail, error)
}

// NetworkingNeutron describes the Neutron methods needed for Networking.
type NetworkingNeutron interface {
	AllocateFloatingIPV2(string) (*neutron.FloatingIPV2, error)
	CreatePortV2(neutron.PortV2) (*neutron.PortV2, error)
	DeletePortV2(string) error
	ListPortsV2(filter ...*neutron.Filter) ([]neutron.PortV2, error)
	GetNetworkV2(string) (*neutron.NetworkV2, error)
	ListFloatingIPsV2(...*neutron.Filter) ([]neutron.FloatingIPV2, error)
	ListNetworksV2(...*neutron.Filter) ([]neutron.NetworkV2, error)
	ListSubnetsV2() ([]neutron.SubnetV2, error)
}

// NetworkingEnvironConfig describes the environConfig methods needed for
// Networking.
type NetworkingEnvironConfig interface {
	networks() []string
	externalNetwork() string
}

// NetworkingAuthenticatingClient describes the AuthenticatingClient methods
// needed for Networking.
type NetworkingAuthenticatingClient interface {
	TenantId() string
}
