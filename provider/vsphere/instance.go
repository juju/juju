// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo
// +build !go1.2 go1.3

package vsphere

import (
	"github.com/juju/errors"
	"github.com/juju/govmomi/vim25/mo"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type environInstance struct {
	base *mo.VirtualMachine
	env  *environ
}

var _ instance.Instance = (*environInstance)(nil)

func newInstance(base *mo.VirtualMachine, env *environ) *environInstance {
	return &environInstance{
		base: base,
		env:  env,
	}
}

// Id implements instance.Instance.
func (inst *environInstance) Id() instance.Id {
	return instance.Id(inst.base.Name)
}

// Status implements instance.Instance.
func (inst *environInstance) Status() string {
	//return inst.base.Status()
	return ""
}

// Refresh implements instance.Instance.
func (inst *environInstance) Refresh() error {
	env := inst.env.getSnapshot()
	err := env.client.Refresh(inst.base)
	return errors.Trace(err)
}

// Addresses implements instance.Instance.
func (inst *environInstance) Addresses() ([]network.Address, error) {
	if inst.base.Guest == nil || inst.base.Guest.IpAddress == "" {
		return nil, nil
	}
	return network.NewAddresses(inst.base.Guest.IpAddress), nil
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
	return errors.Trace(errors.NotImplementedf("OpenPorts"))
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineID string, ports []network.PortRange) error {
	return errors.Trace(errors.NotImplementedf("ClosePorts"))
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) Ports(machineID string) ([]network.PortRange, error) {
	return nil, errors.Trace(errors.NotImplementedf("Ports"))
}
