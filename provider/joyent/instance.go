// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/joyent/gosdc/cloudapi"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

type joyentInstance struct {
	machine *cloudapi.Machine
	env     *joyentEnviron
}

var _ instances.Instance = (*joyentInstance)(nil)

func (inst *joyentInstance) Id() instance.Id {
	return instance.Id(inst.machine.Id)
}

func (inst *joyentInstance) Status(ctx context.ProviderCallContext) instance.Status {
	instStatus := inst.machine.State
	var jujuStatus status.Status
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
	return instance.Status{
		Status:  jujuStatus,
		Message: instStatus,
	}
}

func (inst *joyentInstance) Addresses(ctx context.ProviderCallContext) (network.ProviderAddresses, error) {
	addresses := make([]network.ProviderAddress, 0, len(inst.machine.IPs))
	for _, ip := range inst.machine.IPs {
		address := network.NewProviderAddress(ip)
		if ip == inst.machine.PrimaryIP {
			address.Scope = network.ScopePublic
		} else {
			address.Scope = network.ScopeCloudLocal
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
}
