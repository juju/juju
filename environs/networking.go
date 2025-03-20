// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/envcontext"
)

// SupportsNetworking is a convenience helper to check if an environment
// supports networking. It returns an interface containing Environ and
// Networking in this case.
var SupportsNetworking = supportsNetworking

// DefaultSpaceInfo should be passed into Networking.ProviderSpaceInfo
// to get information about the default space.
var DefaultSpaceInfo *network.SpaceInfo

// Networking interface defines methods that environments
// with networking capabilities must implement.
type Networking interface {
	// Subnets returns basic information about subnets known
	// by the provider for the environment.
	Subnets(
		ctx envcontext.ProviderCallContext, inst instance.Id, subnetIds []network.Id,
	) ([]network.SubnetInfo, error)

	// NetworkInterfaces returns a slice with the network interfaces that
	// correspond to the given instance IDs. If no instances where found,
	// but there was no other error, it will return ErrNoInstances. If some
	// but not all of the instances were found, the returned slice will
	// have some nil slots, and an ErrPartialInstances error will be
	// returned.
	NetworkInterfaces(ctx envcontext.ProviderCallContext, ids []instance.Id) ([]network.InterfaceInfos, error)

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
	Spaces(ctx envcontext.ProviderCallContext) (network.SpaceInfos, error)

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
	ProviderSpaceInfo(ctx envcontext.ProviderCallContext, space *network.SpaceInfo) (*ProviderSpaceInfo, error)

	// SupportsContainerAddresses returns true if the current environment is
	// able to allocate addresses for containers. If returning false, we also
	// return an IsNotSupported error.
	SupportsContainerAddresses(ctx context.Context) (bool, error)

	// AllocateContainerAddresses allocates a static address for each of the
	// container NICs in preparedInfo, hosted by the hostInstanceID. Returns the
	// network config including all allocated addresses on success.
	AllocateContainerAddresses(
		ctx context.Context,
		hostInstanceID instance.Id,
		containerTag names.MachineTag,
		preparedInfo network.InterfaceInfos,
	) (network.InterfaceInfos, error)

	// ReleaseContainerAddresses releases the previously allocated
	// addresses matching the interface details passed in.
	ReleaseContainerAddresses(ctx envcontext.ProviderCallContext, interfaces []network.ProviderInterfaceInfo) error
}

// NetworkingEnviron combines the standard Environ interface with the
// functionality for networking.
type NetworkingEnviron interface {
	// Environ represents a juju environment.
	Environ

	// Networking defines the methods of networking capable environments.
	Networking
}

// NoSpaceDiscoveryEnviron implements methods from Networking that represent an
// environ without native space support (all but MAAS at the time of writing).
// None of the method receiver references are used, so it can be embedded
// as nil without fear of panics.
type NoSpaceDiscoveryEnviron struct{}

// SupportsSpaceDiscovery (Networking) indicates that
// this environ does not support space discovery.
func (*NoSpaceDiscoveryEnviron) SupportsSpaceDiscovery() (bool, error) {
	return false, nil
}

// Spaces (Networking) indicates that this provider
// does not support returning spaces.
func (*NoSpaceDiscoveryEnviron) Spaces(envcontext.ProviderCallContext) (network.SpaceInfos, error) {
	return nil, errors.NotSupportedf("Spaces")
}

// ProviderSpaceInfo (Networking) indicates that this provider
// does not support returning provider info for the input space.
func (*NoSpaceDiscoveryEnviron) ProviderSpaceInfo(
	envcontext.ProviderCallContext, *network.SpaceInfo,
) (*ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("ProviderSpaceInfo")
}

// NoContainerAddressesEnviron implements methods from Networking that represent
// an environ without the ability to allocate container addresses.
// As with NoSpaceDiscoveryEnviron it can be embedded safely.
type NoContainerAddressesEnviron struct{}

// SupportsContainerAddresses (Networking) indicates that this provider does not
// support container addresses.
func (*NoContainerAddressesEnviron) SupportsContainerAddresses(context.Context) (bool, error) {
	return false, nil
}

// AllocateContainerAddresses (Networking) indicates that this provider does
// not support allocating container addresses.
func (*NoContainerAddressesEnviron) AllocateContainerAddresses(
	context.Context, instance.Id, names.MachineTag, network.InterfaceInfos,
) (network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("AllocateContainerAddresses")
}

// ReleaseContainerAddresses (Networking) indicates that this provider does not
// support releasing container addresses.
func (*NoContainerAddressesEnviron) ReleaseContainerAddresses(
	envcontext.ProviderCallContext, []network.ProviderInterfaceInfo,
) error {
	return errors.NotSupportedf("ReleaseContainerAddresses")
}

// SupportsSpaces checks if the environment supports spaces.
func SupportsSpaces(env NetworkingEnviron) bool {
	ok, err := env.SupportsSpaces()
	if err != nil {
		if !errors.Is(err, errors.NotSupported) {
			logger.Errorf(context.TODO(), "checking model spaces support failed with: %v", err)
		}
		return false
	}
	return ok
}

// SupportsContainerAddresses checks if the environment will let us allocate
// addresses for containers from the host ranges.
func SupportsContainerAddresses(ctx context.Context, env BootstrapEnviron) bool {
	netEnv, ok := supportsNetworking(env)
	if !ok {
		return false
	}
	ok, err := netEnv.SupportsContainerAddresses(ctx)
	if err != nil {
		if !errors.Is(err, errors.NotSupported) {
			logger.Errorf(ctx, "checking model container address support failed with: %v", err)
		}
		return false
	}
	return ok
}

func supportsNetworking(environ BootstrapEnviron) (NetworkingEnviron, bool) {
	ne, ok := environ.(NetworkingEnviron)
	return ne, ok
}

// ProviderSpaceInfo contains all the information about a space needed
// by another environ to decide whether it can be routed to.
type ProviderSpaceInfo struct {
	network.SpaceInfo

	// Cloud type governs what attributes will exist in the
	// provider-specific map.
	CloudType string

	// Any provider-specific information to needed to identify the
	// network within the cloud, e.g. VPC ID for EC2.
	ProviderAttributes map[string]interface{}
}
