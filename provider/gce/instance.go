// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type environInstance struct {
	id  instance.Id
	env *environ
}

var _ instance.Instance = (*environInstance)(nil)

func (inst *environInstance) Id() instance.Id {
	return inst.id
}

func (inst *environInstance) Status() string {
	_ = inst.env.getSnapshot()
	return "unknown (not implemented)"
}

func (inst *environInstance) Refresh() error {
	_ = inst.env.getSnapshot()
	return errNotImplemented
}

func (inst *environInstance) Addresses() ([]network.Address, error) {
	_ = inst.env.getSnapshot()
	return nil, errNotImplemented
}

// firewall stuff

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	return errNotImplemented
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	return errNotImplemented
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The ports are returned as sorted by SortPorts.
func (inst *environInstance) Ports(machineId string) ([]network.PortRange, error) {
	return nil, errNotImplemented
}
