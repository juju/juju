// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

var _ environs.NetworkingEnviron = &manualEnviron{}

// SupportsSpaces implements environs.NetworkingEnviron.
func (e *manualEnviron) SupportsSpaces(context.ProviderCallContext) (bool, error) {
	return true, nil
}

// Subnets implements environs.NetworkingEnviron.
func (e *manualEnviron) Subnets(context.ProviderCallContext, instance.Id, []network.Id) ([]network.SubnetInfo, error) {
	return nil, errors.NotSupportedf("subnets")
}

// SuperSubnets implements environs.NetworkingEnviron.
func (e *manualEnviron) SuperSubnets(context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

// SupportsSpaceDiscovery implements environs.NetworkingEnviron.
func (e *manualEnviron) SupportsSpaceDiscovery(context.ProviderCallContext) (bool, error) {
	return false, nil
}

// Spaces implements environs.NetworkingEnviron.
func (e *manualEnviron) Spaces(context.ProviderCallContext) ([]network.SpaceInfo, error) {
	return nil, errors.NotSupportedf("spaces")
}

// SupportsContainerAddresses implements environs.NetworkingEnviron.
func (e *manualEnviron) SupportsContainerAddresses(context.ProviderCallContext) (bool, error) {
	return false, nil
}

// AllocateContainerAddresses implements environs.NetworkingEnviron.
func (e *manualEnviron) AllocateContainerAddresses(
	context.ProviderCallContext, instance.Id, names.MachineTag, network.InterfaceInfos,
) (network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("container addresses")
}

// ReleaseContainerAddresses implements environs.NetworkingEnviron.
func (e *manualEnviron) ReleaseContainerAddresses(context.ProviderCallContext, []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container addresses")
}

// ProviderSpaceInfo implements environs.NetworkingEnviron.
func (*manualEnviron) ProviderSpaceInfo(
	context.ProviderCallContext, *network.SpaceInfo,
) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// AreSpacesRoutable implements environs.NetworkingEnviron.
func (*manualEnviron) AreSpacesRoutable(_ context.ProviderCallContext, _, _ *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// SSHAddresses implements environs.NetworkingEnviron.
func (*manualEnviron) SSHAddresses(
	_ context.ProviderCallContext, addresses network.SpaceAddresses) (network.SpaceAddresses, error) {
	return addresses, nil
}

// NetworkInterfaces implements environs.NetworkingEnviron.
func (e *manualEnviron) NetworkInterfaces(
	context.ProviderCallContext, []instance.Id,
) ([]network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("network interfaces")
}
