// Copyright 2011, 2012, 2013, 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"io"
	"os"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

// Networking defines the methods of networking capable environments have
// to provide.
type Networking interface {
	// AllocateAddress requests a specific address to be allocated for the
	// given instance on the given network.
	AllocateAddress(instId instance.Id, netId network.Id, addr network.Address) error

	// ReleaseAddress releases a specific address previously allocated with
	// AllocateAddress.
	ReleaseAddress(instId instance.Id, netId network.Id, addr network.Address) error

	// Subnets returns basic information about subnets known
	// by the provider for the environment.
	Subnets(inst instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error)

	// NetworkInterfaces requests information about the network
	// interfaces on the given instance.
	NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error)
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
	return environ.(NetworkingEnviron)
}
