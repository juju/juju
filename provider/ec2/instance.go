// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/juju/juju/v2/core/instance"
	"github.com/juju/juju/v2/core/network"
	"github.com/juju/juju/v2/core/network/firewall"
	"github.com/juju/juju/v2/core/status"
	"github.com/juju/juju/v2/environs/config"
	"github.com/juju/juju/v2/environs/context"
	"github.com/juju/juju/v2/environs/instances"
)

// AWS SDK version of instances.Instance.
type sdkInstance struct {
	e *environ
	i types.Instance
}

var _ instances.Instance = (*sdkInstance)(nil)

// String returns a string representation of this instance (the ID).
func (inst *sdkInstance) String() string {
	return string(inst.Id())
}

// Id returns the EC2 identifier for the Instance.
func (inst *sdkInstance) Id() instance.Id {
	return instance.Id(*inst.i.InstanceId)
}

// AvailabilityZone returns the underlying az for an instance.
func (inst *sdkInstance) AvailabilityZone() (string, bool) {
	if inst.i.Placement == nil ||
		inst.i.Placement.AvailabilityZone == nil {
		return "", false
	}
	return *inst.i.Placement.AvailabilityZone, true
}

// Status returns the status of this EC2 instance.
func (inst *sdkInstance) Status(_ context.ProviderCallContext) instance.Status {
	if inst.i.State == nil || inst.i.State.Name == "" {
		return instance.Status{Status: status.Empty}
	}

	// pending | running | shutting-down | terminated | stopping | stopped
	var jujuStatus status.Status
	switch inst.i.State.Name {
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
		Message: string(inst.i.State.Name),
	}
}

// Addresses implements network.Addresses() returning generic address
// details for the instance, and requerying the ec2 api if required.
func (inst *sdkInstance) Addresses(_ context.ProviderCallContext) (network.ProviderAddresses, error) {
	var addresses []network.ProviderAddress
	if inst.i.PublicIpAddress != nil {
		addresses = append(addresses, network.ProviderAddress{
			MachineAddress: network.MachineAddress{
				Value: *inst.i.PublicIpAddress,
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
		})
	}
	if inst.i.PrivateIpAddress != nil {
		addresses = append(addresses, network.ProviderAddress{
			MachineAddress: network.MachineAddress{
				Value: *inst.i.PrivateIpAddress,
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			},
		})
	}
	return addresses, nil
}

// OpenPorts implements instances.InstanceFirewaller.
func (inst *sdkInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
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

// ClosePorts implements instances.InstanceFirewaller.
func (inst *sdkInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, ports firewall.IngressRules) error {
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

// IngressRules implements instances.InstanceFirewaller.
func (inst *sdkInstance) IngressRules(ctx context.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
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
