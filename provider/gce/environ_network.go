// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// TODO(ericsnow) Fold this back into environ.go if neither ends up too big.

// AllocateAddress requests a specific address to be allocated for the
// given instance on the given network.
func (env *environ) AllocateAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return errNotImplemented
}

func (env *environ) ReleaseAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return errNotImplemented
}

func (env *environ) Subnets(inst instance.Id) ([]network.BasicInfo, error) {
	return nil, errNotImplemented
}

func (env *environ) ListNetworks(inst instance.Id) ([]network.BasicInfo, error) {
	return nil, errNotImplemented
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.PortRange) error {
	return errNotImplemented
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.PortRange) error {
	return errNotImplemented
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) Ports() ([]network.PortRange, error) {
	return nil, errNotImplemented
}
