// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// Networking interface defines methods that environments
// with networking capabilities must implement.
type Networking interface {
	// AllocateAddress requests a specific address to be allocated for the
	// given instance on the given subnet.
	AllocateAddress(instId instance.Id, subnetId network.Id, addr network.Address, macAddress, hostname string) error

	// ReleaseAddress releases a specific address previously allocated with
	// AllocateAddress.
	ReleaseAddress(instId instance.Id, subnetId network.Id, addr network.Address, macAddress string) error

	// Subnets returns basic information about subnets known
	// by the provider for the environment.
	Subnets(inst instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error)

	// NetworkInterfaces requests information about the network
	// interfaces on the given instance.
	NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error)

	// SupportsAddressAllocation returns whether the given subnetId
	// supports static IP address allocation using AllocateAddress and
	// ReleaseAddress. If subnetId is network.AnySubnet, the provider
	// can decide whether it can return true or a false and an error
	// (e.g. "subnetId must be set").
	SupportsAddressAllocation(subnetId network.Id) (bool, error)
}

// NetworkingEnviron combines the standard Environ interface with the
// functionality for networking.
type NetworkingEnviron interface {
	// Environ represents a juju environment.
	Environ

	// Networking defines the methods of networking capable environments.
	Networking
}

// SupportsNetworking is a convenience helper to check if an environment
// supports networking. It returns an interface containing Environ and
// Networking in this case.
func SupportsNetworking(environ Environ) (NetworkingEnviron, bool) {
	ne, ok := environ.(NetworkingEnviron)
	return ne, ok
}

// AddressAllocationEnabled is a shortcut for checking if the
// AddressAllocation feature flag is enabled.
func AddressAllocationEnabled() bool {
	return featureflag.Enabled(feature.AddressAllocation)
}
