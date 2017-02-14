// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
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
	jujuStatus := status.Provisioning
	switch instStatus {
	case "PROVISIONING", "STAGING":
		jujuStatus = status.Provisioning
	case "RUNNING":
		jujuStatus = status.Running
	case "STOPPING", "TERMINATED":
		jujuStatus = status.Empty
	default:
		jujuStatus = status.Empty
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
func (inst *environInstance) OpenPorts(machineID string, rules []network.IngressRule) error {
	// TODO(ericsnow) Make sure machineId matches inst.Id()?
	name, err := inst.env.namespace.Hostname(machineID)
	if err != nil {
		return errors.Trace(err)
	}
	err = inst.env.gce.OpenPorts(name, rules...)
	return errors.Trace(err)
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineID string, rules []network.IngressRule) error {
	name, err := inst.env.namespace.Hostname(machineID)
	if err != nil {
		return errors.Trace(err)
	}
	err = inst.env.gce.ClosePorts(name, rules...)
	return errors.Trace(err)
}

// IngressRules returns the set of ingress rules applicable to the instance, which
// should have been started with the given machine id.
// The rules are returned as sorted by SortIngressRules.
func (inst *environInstance) IngressRules(machineID string) ([]network.IngressRule, error) {
	name, err := inst.env.namespace.Hostname(machineID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ports, err := inst.env.gce.IngressRules(name)
	return ports, errors.Trace(err)
}
