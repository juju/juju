// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"gopkg.in/amz.v3/ec2"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
)

type ec2Instance struct {
	e *environ

	*ec2.Instance
}

func (inst *ec2Instance) String() string {
	return string(inst.Id())
}

var _ instance.Instance = (*ec2Instance)(nil)

func (inst *ec2Instance) Id() instance.Id {
	return instance.Id(inst.InstanceId)
}

func (inst *ec2Instance) Status() instance.InstanceStatus {
	// pending | running | shutting-down | terminated | stopping | stopped
	jujuStatus := status.StatusPending
	switch inst.State.Name {
	case "pending":
		jujuStatus = status.StatusPending
	case "running":
		jujuStatus = status.StatusRunning
	case "shutting-down", "terminated", "stopping", "stopped":
		jujuStatus = status.StatusEmpty
	default:
		jujuStatus = status.StatusEmpty
	}
	return instance.InstanceStatus{
		Status:  jujuStatus,
		Message: inst.State.Name,
	}

}

// Addresses implements network.Addresses() returning generic address
// details for the instance, and requerying the ec2 api if required.
func (inst *ec2Instance) Addresses() ([]network.Address, error) {
	var addresses []network.Address
	possibleAddresses := []network.Address{
		{
			Value: inst.IPAddress,
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
		},
		{
			Value: inst.PrivateIPAddress,
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
	}
	for _, address := range possibleAddresses {
		if address.Value != "" {
			addresses = append(addresses, address)
		}
	}
	return addresses, nil
}

func (inst *ec2Instance) OpenPorts(machineId string, ports []network.PortRange) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.openPortsInGroup(name, ports); err != nil {
		return err
	}
	logger.Infof("opened ports in security group %s: %v", name, ports)
	return nil
}

func (inst *ec2Instance) ClosePorts(machineId string, ports []network.PortRange) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.closePortsInGroup(name, ports); err != nil {
		return err
	}
	logger.Infof("closed ports in security group %s: %v", name, ports)
	return nil
}

func (inst *ec2Instance) Ports(machineId string) ([]network.PortRange, error) {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	ranges, err := inst.e.portsInGroup(name)
	if err != nil {
		return nil, err
	}
	return ranges, nil
}
