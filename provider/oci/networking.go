// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// Subnets is defined on the environs.Networking interface.
func (e *Environ) Subnets(ctx context.ProviderCallContext, id instance.Id, subnets []network.Id) ([]network.SubnetInfo, error) {
	return nil, nil
}

func (e *Environ) SuperSubnets(ctx context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

func (e *Environ) NetworkInterfaces(ctx context.ProviderCallContext, instId instance.Id) ([]network.InterfaceInfo, error) {
	return nil, errors.NotImplementedf("NetworkInterfaces")
}

func (e *Environ) SupportsSpaces(ctx context.ProviderCallContext) (bool, error) {
	return false, errors.NotSupportedf("spaces")
}

func (e *Environ) SupportsSpaceDiscovery(ctx context.ProviderCallContext) (bool, error) {
	return false, errors.NotSupportedf("space discovery")
}

func (e *Environ) Spaces(ctx context.ProviderCallContext) ([]network.SpaceInfo, error) {
	return nil, errors.NotImplementedf("Spaces")
}

func (e *Environ) ProviderSpaceInfo(ctx context.ProviderCallContext, space *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotImplementedf("ProviderSpaceInfo")
}

func (e *Environ) AreSpacesRoutable(ctx context.ProviderCallContext, space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, errors.NotImplementedf("AreSpacesRoutable")
}

func (e *Environ) SupportsContainerAddresses(ctx context.ProviderCallContext) (bool, error) {
	return false, errors.NotSupportedf("container addresses")
}

func (e *Environ) AllocateContainerAddresses(
	ctx context.ProviderCallContext,
	hostInstanceID instance.Id,
	containerTag names.MachineTag,
	preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {

	return nil, errors.NotImplementedf("AllocateContainerAddresses")
}

func (e *Environ) ReleaseContainerAddresses(ctx context.ProviderCallContext, interfaces []network.ProviderInterfaceInfo) error {
	return errors.NotImplementedf("ReleaseContainerAddresses")
}

func (e *Environ) SSHAddresses(ctx context.ProviderCallContext, addresses []network.Address) ([]network.Address, error) {
	return addresses, errors.NotImplementedf("SSHAddresses")
}
