// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// SupportsNetworking is a convenience helper to check if an environment
// supports networking. It returns an interface containing Environ and
// Networking in this case.
var SupportsNetworking = supportsNetworking

// DefaultProviderSpaceId is the provider id for the default space.
const DefaultProviderSpaceId = ""

// Networking interface defines methods that environments
// with networking capabilities must implement.
type Networking interface {
	// Subnets returns basic information about subnets known
	// by the provider for the environment.
	Subnets(inst instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error)

	// NetworkInterfaces requests information about the network
	// interfaces on the given instance.
	NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error)

	// SupportsSpaces returns whether the current environment supports
	// spaces. The returned error satisfies errors.IsNotSupported(),
	// unless a general API failure occurs.
	SupportsSpaces() (bool, error)

	// SupportsSpaceDiscovery returns whether the current environment
	// supports discovering spaces from the provider. The returned error
	// satisfies errors.IsNotSupported(), unless a general API failure occurs.
	SupportsSpaceDiscovery() (bool, error)

	// Spaces returns a slice of network.SpaceInfo with info, including
	// details of all associated subnets, about all spaces known to the
	// provider that have subnets available.
	Spaces() ([]network.SpaceInfo, error)

	// ProviderSpaceInfo returns the details of the space requested as
	// a ProviderSpaceInfo. This will contain everything needed to
	// decide whether an Environ of the same type in another
	// controller could route to the space. Details for the default
	// space can be retrieved by specifying DefaultProviderSpaceId.
	// This method can often be implemented even if the provider
	// doesn't support spaces fully.
	ProviderSpaceInfo(providerSpaceId string) (*ProviderSpaceInfo, error)

	// IsRoutable returns whether this Environ can connect to the
	// given space via cloud-local addresses.
	IsSpaceRoutable(targetSpace *ProviderSpaceInfo) (bool, error)

	// SupportsContainerAddresses returns true if the current environment is
	// able to allocate addresses for containers. If returning false, we also
	// return an IsNotSupported error.
	SupportsContainerAddresses() (bool, error)

	// AllocateContainerAddresses allocates a static address for each of the
	// container NICs in preparedInfo, hosted by the hostInstanceID. Returns the
	// network config including all allocated addresses on success.
	AllocateContainerAddresses(hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error)

	// ReleaseContainerAddresses releases the previously allocated
	// addresses matching the interface details passed in.
	ReleaseContainerAddresses(interfaces []network.ProviderInterfaceInfo) error
}

// NetworkingEnviron combines the standard Environ interface with the
// functionality for networking.
type NetworkingEnviron interface {
	// Environ represents a juju environment.
	Environ

	// Networking defines the methods of networking capable environments.
	Networking
}

func supportsNetworking(environ Environ) (NetworkingEnviron, bool) {
	ne, ok := environ.(NetworkingEnviron)
	return ne, ok
}

// SupportsSpaces checks if the environment implements NetworkingEnviron
// and also if it supports spaces.
func SupportsSpaces(env Environ) bool {
	netEnv, ok := supportsNetworking(env)
	if !ok {
		return false
	}
	ok, err := netEnv.SupportsSpaces()
	if err != nil {
		if !errors.IsNotSupported(err) {
			logger.Errorf("checking model spaces support failed with: %v", err)
		}
		return false
	}
	return ok
}

// SupportsContainerAddresses checks if the environment will let us allocate
// addresses for containers from the host ranges.
func SupportsContainerAddresses(env Environ) bool {
	netEnv, ok := supportsNetworking(env)
	if !ok {
		return false
	}
	ok, err := netEnv.SupportsContainerAddresses()
	if err != nil {
		if !errors.IsNotSupported(err) {
			logger.Errorf("checking model container address support failed with: %v", err)
		}
		return false
	}
	return ok
}

// ProviderSpaceInfo contains all the information about a space needed
// by another environ to decide whether it can be routed to.
type ProviderSpaceInfo struct {
	network.SpaceInfo

	// This identifies the cloud containing the space. Care must be
	// taken by providers that the credential information doesn't
	// include any secrets - this struct may be sent outside the
	// local controller.
	CloudSpec CloudSpec

	// Any provider-specific information to needed to identify the
	// network within the cloud, e.g. VPC ID for EC2.
	ProviderAttributes map[string]interface{}
}
