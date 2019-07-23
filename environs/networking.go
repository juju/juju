// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
)

// SupportsNetworking is a convenience helper to check if an environment
// supports networking. It returns an interface containing Environ and
// Networking in this case.
var SupportsNetworking = supportsNetworking

// DefaultSpaceInfo should be passed into Networking.ProviderSpaceInfo
// to get information about the default space.
var DefaultSpaceInfo *corenetwork.SpaceInfo

// Networking interface defines methods that environments
// with networking capabilities must implement.
type Networking interface {
	// Subnets returns basic information about subnets known
	// by the provider for the environment.
	Subnets(
		ctx context.ProviderCallContext, inst instance.Id, subnetIds []corenetwork.Id,
	) ([]corenetwork.SubnetInfo, error)

	// SuperSubnets returns information about aggregated subnets - eg. global CIDR
	// for EC2 VPC.
	SuperSubnets(ctx context.ProviderCallContext) ([]string, error)

	// NetworkInterfaces requests information about the network
	// interfaces on the given instance.
	NetworkInterfaces(ctx context.ProviderCallContext, instId instance.Id) ([]network.InterfaceInfo, error)

	// SupportsSpaces returns whether the current environment supports
	// spaces. The returned error satisfies errors.IsNotSupported(),
	// unless a general API failure occurs.
	SupportsSpaces(ctx context.ProviderCallContext) (bool, error)

	// SupportsSpaceDiscovery returns whether the current environment
	// supports discovering spaces from the provider. The returned error
	// satisfies errors.IsNotSupported(), unless a general API failure occurs.
	SupportsSpaceDiscovery(ctx context.ProviderCallContext) (bool, error)

	// Spaces returns a slice of network.SpaceInfo with info, including
	// details of all associated subnets, about all spaces known to the
	// provider that have subnets available.
	Spaces(ctx context.ProviderCallContext) ([]corenetwork.SpaceInfo, error)

	// ProviderSpaceInfo returns the details of the space requested as
	// a ProviderSpaceInfo. This will contain everything needed to
	// decide whether an Environ of the same type in another
	// controller could route to the space. Details for the default
	// space can be retrieved by passing DefaultSpaceInfo (which is nil).
	//
	// This method accepts a SpaceInfo with details of the space that
	// we need provider details for - this is the Juju model's view of
	// what subnets are in the space. If the provider supports spaces
	// and space discovery then it is the authority on what subnets
	// are actually in the space, and it's free to collect the full
	// space and subnet info using the space's ProviderId (discarding
	// the subnet details passed in which might be out-of date).
	//
	// If the provider doesn't support space discovery then the Juju
	// model's opinion of what subnets are in the space is
	// authoritative. In that case the provider should collect up any
	// other information needed to determine routability and include
	// the passed-in space info in the ProviderSpaceInfo returned.
	ProviderSpaceInfo(ctx context.ProviderCallContext, space *corenetwork.SpaceInfo) (*ProviderSpaceInfo, error)

	// AreSpacesRoutable returns whether the communication between the
	// two spaces can use cloud-local addresses.
	AreSpacesRoutable(ctx context.ProviderCallContext, space1, space2 *ProviderSpaceInfo) (bool, error)

	// SupportsContainerAddresses returns true if the current environment is
	// able to allocate addresses for containers. If returning false, we also
	// return an IsNotSupported error.
	SupportsContainerAddresses(ctx context.ProviderCallContext) (bool, error)

	// AllocateContainerAddresses allocates a static address for each of the
	// container NICs in preparedInfo, hosted by the hostInstanceID. Returns the
	// network config including all allocated addresses on success.
	AllocateContainerAddresses(ctx context.ProviderCallContext, hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error)

	// ReleaseContainerAddresses releases the previously allocated
	// addresses matching the interface details passed in.
	ReleaseContainerAddresses(ctx context.ProviderCallContext, interfaces []network.ProviderInterfaceInfo) error

	// SSHAddresses filters provided addresses for addresses usable for SSH
	SSHAddresses(ctx context.ProviderCallContext, addresses []network.Address) ([]network.Address, error)
}

// NetworkingEnviron combines the standard Environ interface with the
// functionality for networking.
type NetworkingEnviron interface {
	// Environ represents a juju environment.
	Environ

	// Networking defines the methods of networking capable environments.
	Networking
}

func supportsNetworking(environ BootstrapEnviron) (NetworkingEnviron, bool) {
	ne, ok := environ.(NetworkingEnviron)
	return ne, ok
}

// SupportsSpaces checks if the environment implements NetworkingEnviron
// and also if it supports spaces.
func SupportsSpaces(ctx context.ProviderCallContext, env BootstrapEnviron) bool {
	netEnv, ok := supportsNetworking(env)
	if !ok {
		return false
	}
	ok, err := netEnv.SupportsSpaces(ctx)
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
func SupportsContainerAddresses(ctx context.ProviderCallContext, env BootstrapEnviron) bool {
	netEnv, ok := supportsNetworking(env)
	if !ok {
		return false
	}
	ok, err := netEnv.SupportsContainerAddresses(ctx)
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
	corenetwork.SpaceInfo

	// Cloud type governs what attributes will exist in the
	// provider-specific map.
	CloudType string

	// Any provider-specific information to needed to identify the
	// network within the cloud, e.g. VPC ID for EC2.
	ProviderAttributes map[string]interface{}
}
