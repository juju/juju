// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// Subnets is defined on the environs.Networking interface.
func (e *Environ) Subnets(id instance.Id, subnets []network.Id) ([]network.SubnetInfo, error) {
	return nil, nil
}

func (e *Environ) SuperSubnets() ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

func (e *Environ) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	return nil, nil
}

func (e *Environ) SupportsSpaces() (bool, error) {
	return false, errors.NotSupportedf("spaces")
}

func (e *Environ) SupportsSpaceDiscovery() (bool, error) {
	return false, errors.NotSupportedf("space discovery")
}

func (e *Environ) Spaces() ([]network.SpaceInfo, error) {
	return nil, nil
}

func (e *Environ) ProviderSpaceInfo(space *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	return nil, nil
}

func (e *Environ) AreSpacesRoutable(space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

func (e *Environ) SupportsContainerAddresses() (bool, error) {
	return false, errors.NotSupportedf("container addresses")
}

func (e *Environ) AllocateContainerAddresses(
	hostInstanceID instance.Id,
	containerTag names.MachineTag,
	preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {

	return nil, nil
}

func (e *Environ) ReleaseContainerAddresses(interfaces []network.ProviderInterfaceInfo) error {
	return nil
}

func (e *Environ) SSHAddresses(addresses []network.Address) ([]network.Address, error) {
	return addresses, nil
}
