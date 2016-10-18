// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/joyent/gosdc/cloudapi"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
)

type joyentInstance struct {
	machine *cloudapi.Machine
	env     *joyentEnviron
}

var _ instance.Instance = (*joyentInstance)(nil)

func (inst *joyentInstance) Id() instance.Id {
	return instance.Id(inst.machine.Id)
}

func (inst *joyentInstance) Status() instance.InstanceStatus {
	instStatus := inst.machine.State
	jujuStatus := status.Pending
	switch instStatus {
	case "configured", "incomplete", "unavailable", "provisioning":
		jujuStatus = status.Allocating
	case "ready", "running":
		jujuStatus = status.Running
	case "halting", "stopping", "shutting_down", "off", "down", "installed", "stopped", "destroyed", "unreachable":
		jujuStatus = status.Empty
	case "failed":
		jujuStatus = status.ProvisioningError
	default:
		jujuStatus = status.Empty
	}
	return instance.InstanceStatus{
		Status:  jujuStatus,
		Message: instStatus,
	}
}

func (inst *joyentInstance) Addresses() ([]network.Address, error) {
	addresses := make([]network.Address, 0, len(inst.machine.IPs))
	for _, ip := range inst.machine.IPs {
		address := network.NewAddress(ip)
		if ip == inst.machine.PrimaryIP {
			address.Scope = network.ScopePublic
		} else {
			address.Scope = network.ScopeCloudLocal
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
}
