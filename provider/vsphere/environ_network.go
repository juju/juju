// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo
// +build !go1.2 go1.3

package vsphere

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// AllocateAddress implements environs.Environ, but is not implmemented.
func (env *environ) AllocateAddress(instID instance.Id, netID network.Id, addr network.Address) error {
	//TODO: implement
	return errors.Trace(errors.NotImplementedf(""))
}

// ReleaseAddress implements environs.Environ, but is not implmemented.
func (env *environ) ReleaseAddress(instID instance.Id, netID network.Id, addr network.Address) error {
	//TODO: implement
	return errors.Trace(errors.NotImplementedf(""))
}

// Subnets implements environs.Environ, but is not implmemented.
func (env *environ) Subnets(inst instance.Id, ids []network.Id) ([]network.SubnetInfo, error) {
	//TODO: implement
	return nil, errors.Trace(errors.NotImplementedf(""))
}

// ListNetworks implements environs.Environ, but is not implmemented.
func (env *environ) ListNetworks(inst instance.Id) ([]network.SubnetInfo, error) {
	//TODO: implement
	return nil, errors.Trace(errors.NotImplementedf(""))
}

// NetworkInterfaces implements environs.Environ, but is not implmemented.
func (env *environ) NetworkInterfaces(inst instance.Id) ([]network.InterfaceInfo, error) {
	//TODO: implement
	return nil, errors.Trace(errors.NotImplementedf(""))
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.PortRange) error {
	//TODO: implement
	return errors.Trace(errors.NotImplementedf(""))
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.PortRange) error {
	//TODO: implement
	return errors.Trace(errors.NotImplementedf(""))
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) Ports() ([]network.PortRange, error) {
	//TODO: implement
	return nil, errors.Trace(errors.NotImplementedf(""))
}
