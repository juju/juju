// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

// AllocateAddress requests a specific address to be allocated for the
// given instance on the given network.
func (env *environ) AllocateAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return errors.Trace(errNotImplemented)
}

func (env *environ) ReleaseAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return errors.Trace(errNotImplemented)
}

func (env *environ) Subnets(inst instance.Id) ([]network.SubnetInfo, error) {
	return nil, errors.Trace(errNotImplemented)
}

func (env *environ) ListNetworks(inst instance.Id) ([]network.SubnetInfo, error) {
	return nil, errors.Trace(errNotImplemented)
}

func (env *environ) globalFirewallName() string {
	fwName := common.MachineFullName(env, "")
	return fwName[:len(fwName)-1]
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.PortRange) error {
	err := env.gce.OpenPorts(env.globalFirewallName(), ports)
	return errors.Trace(err)
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.PortRange) error {
	err := env.gce.ClosePorts(env.globalFirewallName(), ports)
	return errors.Trace(err)
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) Ports() ([]network.PortRange, error) {
	ports, err := env.gce.Ports(env.globalFirewallName())
	return ports, errors.Trace(err)
}
