// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"gopkg.in/amz.v3/ec2"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

type ec2Instance struct {
	e *environ

	*ec2.Instance
}

func (inst *ec2Instance) String() string {
	return string(inst.Id())
}

var _ instances.Instance = (*ec2Instance)(nil)

func (inst *ec2Instance) Id() instance.Id {
	return instance.Id(inst.InstanceId)
}

func (inst *ec2Instance) Status(ctx context.ProviderCallContext) instance.Status {
	// pending | running | shutting-down | terminated | stopping | stopped
	var jujuStatus status.Status
	switch inst.State.Name {
	case "pending":
		jujuStatus = status.Pending
	case "running":
		jujuStatus = status.Running
	case "shutting-down", "terminated", "stopping", "stopped":
		jujuStatus = status.Empty
	default:
		jujuStatus = status.Empty
	}
	return instance.Status{
		Status:  jujuStatus,
		Message: inst.State.Name,
	}

}

// Addresses implements network.Addresses() returning generic address
// details for the instance, and requerying the ec2 api if required.
func (inst *ec2Instance) Addresses(ctx context.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	var addresses []corenetwork.ProviderAddress
	possibleAddresses := []corenetwork.ProviderAddress{
		{
			MachineAddress: corenetwork.MachineAddress{
				Value: inst.IPAddress,
				Type:  corenetwork.IPv4Address,
				Scope: corenetwork.ScopePublic,
			},
		},
		{
			MachineAddress: corenetwork.MachineAddress{
				Value: inst.PrivateIPAddress,
				Type:  corenetwork.IPv4Address,
				Scope: corenetwork.ScopeCloudLocal,
			},
		},
	}
	for _, address := range possibleAddresses {
		if address.Value != "" {
			addresses = append(addresses, address)
		}
	}
	return addresses, nil
}

func (inst *ec2Instance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.openPortsInGroup(ctx, name, rules); err != nil {
		return err
	}
	logger.Infof("opened ports in security group %s: %v", name, rules)
	return nil
}

func (inst *ec2Instance) ClosePorts(ctx context.ProviderCallContext, machineId string, ports firewall.IngressRules) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.closePortsInGroup(ctx, name, ports); err != nil {
		return err
	}
	logger.Infof("closed ports in security group %s: %v", name, ports)
	return nil
}

func (inst *ec2Instance) IngressRules(ctx context.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ingress rules from instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	ranges, err := inst.e.ingressRulesInGroup(ctx, name)
	if err != nil {
		return nil, err
	}
	return ranges, nil
}
