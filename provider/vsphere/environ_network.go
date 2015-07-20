// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// AllocateAddress implements environs.Environ.
func (env *environ) AllocateAddress(instID instance.Id, subnetID network.Id, addr network.Address, _, _ string) error {
	return env.changeAddress(instID, subnetID, addr, true)
}

// ReleaseAddress implements environs.Environ.
func (env *environ) ReleaseAddress(instID instance.Id, netID network.Id, addr network.Address, _ string) error {
	return env.changeAddress(instID, netID, addr, false)
}

func (env *environ) changeAddress(instID instance.Id, netID network.Id, addr network.Address, add bool) error {
	instances, err := env.Instances([]instance.Id{instID})
	if err != nil {
		return errors.Trace(err)
	}
	inst := instances[0].(*environInstance)
	_, client, err := inst.getSshClient()
	if err != nil {
		return errors.Trace(err)
	}
	interfaceName := "eth0"
	if string(netID) == env.ecfg.externalNetwork() {
		interfaceName = "eth1"
	}
	if add {
		err = client.addIpAddress(interfaceName, addr.Value)
	} else {
		err = client.releaseIpAddress(interfaceName, addr.Value)
	}

	return errors.Trace(err)
}

// SupportsAddressAllocation is specified on environs.Networking.
func (env *environ) SupportsAddressAllocation(_ network.Id) (bool, error) {
	return true, nil
}

// Subnets implements environs.Environ.
func (env *environ) Subnets(inst instance.Id, ids []network.Id) ([]network.SubnetInfo, error) {
	return env.client.Subnets(inst, ids)
}

// NetworkInterfaces implements environs.Environ.
func (env *environ) NetworkInterfaces(inst instance.Id) ([]network.InterfaceInfo, error) {
	return env.client.GetNetworkInterfaces(inst, env.ecfg)
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.PortRange) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.PortRange) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) Ports() ([]network.PortRange, error) {
	return nil, errors.Trace(errors.NotSupportedf("Ports"))
}
