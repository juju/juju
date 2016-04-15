// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/status"
)

type environInstance struct {
	base *google.Instance
	env  *environ
}

var _ instance.Instance = (*environInstance)(nil)

func newInstance(base *google.Instance, env *environ) *environInstance {
	return &environInstance{
		base: base,
		env:  env,
	}
}

// Id implements instance.Instance.
func (inst *environInstance) Id() instance.Id {
	return instance.Id(inst.base.ID)
}

// Status implements instance.Instance.
func (inst *environInstance) Status() instance.InstanceStatus {
	instStatus := inst.base.Status()
	jujuStatus := status.StatusProvisioning
	switch instStatus {
	case "PROVISIONING", "STAGING":
		jujuStatus = status.StatusProvisioning
	case "RUNNING":
		jujuStatus = status.StatusRunning
	case "STOPPING", "TERMINATED":
		jujuStatus = status.StatusEmpty
	default:
		jujuStatus = status.StatusEmpty
	}
	return instance.InstanceStatus{
		Status:  jujuStatus,
		Message: instStatus,
	}
}

// Addresses implements instance.Instance.
func (inst *environInstance) Addresses() ([]network.Address, error) {
	return inst.base.Addresses(), nil
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
	name := common.MachineFullName(inst.env.Config().UUID(), machineID)
	err := inst.env.gce.OpenPorts(name, ports...)
	return errors.Trace(err)
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineID string, ports []network.PortRange) error {
	name := common.MachineFullName(inst.env.Config().UUID(), machineID)
	err := inst.env.gce.ClosePorts(name, ports...)
	return errors.Trace(err)
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The ports are returned as sorted by SortPorts.
func (inst *environInstance) Ports(machineID string) ([]network.PortRange, error) {
	name := common.MachineFullName(inst.env.Config().UUID(), machineID)
	ports, err := inst.env.gce.Ports(name)
	return ports, errors.Trace(err)
}
