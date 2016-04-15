// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/status"
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
func (inst *environInstance) Status() instance.InstanceStatus {
	jujuStatus := status.StatusPending
	instStatus := inst.raw.Status()
	switch instStatus {
	case lxdclient.StatusStarting, lxdclient.StatusStarted:
		jujuStatus = status.StatusAllocating
	case lxdclient.StatusRunning:
		jujuStatus = status.StatusRunning
	case lxdclient.StatusFreezing, lxdclient.StatusFrozen, lxdclient.StatusThawed, lxdclient.StatusStopping, lxdclient.StatusStopped:
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
	return inst.env.raw.Addresses(inst.raw.Name)
}

// firewall stuff

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) OpenPorts(machineID string, ports []network.PortRange) error {
	// TODO(ericsnow) Make sure machineId matches inst.Id()?
	name := common.MachineFullName(inst.env.Config().UUID(), machineID)
	err := inst.env.raw.OpenPorts(name, ports...)
	if errors.IsNotImplemented(err) {
		// TODO(ericsnow) for now...
		return nil
	}
	return errors.Trace(err)
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineID string, ports []network.PortRange) error {
	name := common.MachineFullName(inst.env.Config().UUID(), machineID)
	err := inst.env.raw.ClosePorts(name, ports...)
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
	name := common.MachineFullName(inst.env.Config().UUID(), machineID)
	ports, err := inst.env.raw.Ports(name)
	if errors.IsNotImplemented(err) {
		// TODO(ericsnow) for now...
		return nil, nil
	}
	return ports, errors.Trace(err)
}
