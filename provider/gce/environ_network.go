// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

// AllocateAddress implements environs.Environ, but is not implmemented.
func (env *environ) AllocateAddress(instID instance.Id, netID network.Id, addr network.Address, macAddress, hostname string) error {
	return errors.Trace(errNotImplemented)
}

// ReleaseAddress implements environs.Environ, but is not implmemented.
func (env *environ) ReleaseAddress(instID instance.Id, netID network.Id, addr network.Address, macAddres string) error {
	return errors.Trace(errNotImplemented)
}

// Subnets implements environs.Environ, but is not implmemented.
func (env *environ) Subnets(inst instance.Id, ids []network.Id) ([]network.SubnetInfo, error) {
	return nil, errors.Trace(errNotImplemented)
}

// ListNetworks implements environs.Environ, but is not implmemented.
func (env *environ) ListNetworks(inst instance.Id) ([]network.SubnetInfo, error) {
	return nil, errors.Trace(errNotImplemented)
}

// NetworkInterfaces implements environs.Environ, but is not implmemented.
func (env *environ) NetworkInterfaces(inst instance.Id) ([]network.InterfaceInfo, error) {
	return nil, errors.Trace(errNotImplemented)
}

// globalFirewallName returns the name to use for the global firewall.
func (env *environ) globalFirewallName() string {
	return common.EnvFullName(env)
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.PortRange) error {
	err := env.gce.OpenPorts(env.globalFirewallName(), ports...)
	return errors.Trace(err)
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.PortRange) error {
	err := env.gce.ClosePorts(env.globalFirewallName(), ports...)
	return errors.Trace(err)
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) Ports() ([]network.PortRange, error) {
	ports, err := env.gce.Ports(env.globalFirewallName())
	return ports, errors.Trace(err)
}
