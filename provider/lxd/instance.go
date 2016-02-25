// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/tools/lxdclient"
)

type environInstance struct {
	raw *lxdclient.Instance
	env *environ
}

var _ instance.Instance = (*environInstance)(nil)

func newInstance(raw *lxdclient.Instance, env *environ) *environInstance {
	return &environInstance{
		raw: raw,
		env: env,
	}
}

// Id implements instance.Instance.
func (inst *environInstance) Id() instance.Id {
	return instance.Id(inst.raw.Name)
}

// Status implements instance.Instance.
func (inst *environInstance) Status() string {
	return inst.raw.Status()
}

// Addresses implements instance.Instance.
func (inst *environInstance) Addresses() ([]network.Address, error) {
	return inst.env.raw.Addresses(inst.raw.Name)
}

func findInst(id instance.Id, instances []instance.Instance) instance.Instance {
	for _, inst := range instances {
		if id == inst.Id() {
			return inst
		}
	}
	return nil
}

// firewall stuff

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) OpenPorts(machineID string, ports []network.PortRange) error {
	// TODO(ericsnow) Make sure machineId matches inst.Id()?
	name := common.MachineFullName(inst.env, machineID)
	env := inst.env.getSnapshot()
	err := env.raw.OpenPorts(name, ports...)
	if errors.IsNotImplemented(err) {
		// TODO(ericsnow) for now...
		return nil
	}
	return errors.Trace(err)
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineID string, ports []network.PortRange) error {
	name := common.MachineFullName(inst.env, machineID)
	env := inst.env.getSnapshot()
	err := env.raw.ClosePorts(name, ports...)
	if errors.IsNotImplemented(err) {
		// TODO(ericsnow) for now...
		return nil
	}
	return errors.Trace(err)
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The ports are returned as sorted by SortPorts.
func (inst *environInstance) Ports(machineID string) ([]network.PortRange, error) {
	name := common.MachineFullName(inst.env, machineID)
	env := inst.env.getSnapshot()
	ports, err := env.raw.Ports(name)
	if errors.IsNotImplemented(err) {
		// TODO(ericsnow) for now...
		return nil, nil
	}
	return ports, errors.Trace(err)
}
