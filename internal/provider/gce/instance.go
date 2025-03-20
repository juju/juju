// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/gce/google"
)

type environInstance struct {
	base *google.Instance
	env  *environ
}

var _ instances.Instance = (*environInstance)(nil)

func newInstance(base *google.Instance, env *environ) *environInstance {
	return &environInstance{
		base: base,
		env:  env,
	}
}

// Id implements instances.Instance.
func (inst *environInstance) Id() instance.Id {
	return instance.Id(inst.base.ID)
}

// Status implements instances.Instance.
func (inst *environInstance) Status(ctx envcontext.ProviderCallContext) instance.Status {
	instStatus := inst.base.Status()
	var jujuStatus status.Status
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
	return instance.Status{
		Status:  jujuStatus,
		Message: instStatus,
	}
}

// Addresses implements instances.Instance.
func (inst *environInstance) Addresses(ctx envcontext.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	return inst.base.Addresses(), nil
}

func findInst(id instance.Id, instances []instances.Instance) instances.Instance {
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
func (inst *environInstance) OpenPorts(ctx envcontext.ProviderCallContext, machineID string, rules firewall.IngressRules) error {
	// TODO(ericsnow) Make sure machineId matches inst.Id()?
	name, err := inst.env.namespace.Hostname(machineID)
	if err != nil {
		return errors.Trace(err)
	}
	err = inst.env.gce.OpenPorts(name, rules)
	if err != nil {
		return inst.env.HandleCredentialError(ctx, err)
	}
	return nil
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(ctx envcontext.ProviderCallContext, machineID string, rules firewall.IngressRules) error {
	name, err := inst.env.namespace.Hostname(machineID)
	if err != nil {
		return errors.Trace(err)
	}
	err = inst.env.gce.ClosePorts(name, rules)
	if err != nil {
		return inst.env.HandleCredentialError(ctx, err)
	}
	return nil
}

// IngressRules returns the set of ingress rules applicable to the instance, which
// should have been started with the given machine id.
// The rules are returned as sorted by SortIngressRules.
func (inst *environInstance) IngressRules(ctx envcontext.ProviderCallContext, machineID string) (firewall.IngressRules, error) {
	name, err := inst.env.namespace.Hostname(machineID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ports, err := inst.env.gce.IngressRules(name)
	if err != nil {
		return nil, inst.env.HandleCredentialError(ctx, err)
	}
	return ports, nil
}
