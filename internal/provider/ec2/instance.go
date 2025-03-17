// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
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
func (inst *sdkInstance) Status(_ envcontext.ProviderCallContext) instance.Status {
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
func (inst *sdkInstance) Addresses(_ envcontext.ProviderCallContext) (network.ProviderAddresses, error) {
	var addresses []network.ProviderAddress
	if inst.i.Ipv6Address != nil {
		addresses = append(addresses, network.ProviderAddress{
			MachineAddress: network.MachineAddress{
				Value: *inst.i.Ipv6Address,
				Type:  network.IPv6Address,
				Scope: network.ScopePublic,
			},
		})
	}
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
func (inst *sdkInstance) OpenPorts(ctx envcontext.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.openPortsInGroup(ctx, name, rules); err != nil {
		return err
	}
	logger.Infof(ctx, "opened ports in security group %s: %v", name, rules)
	return nil
}

// ClosePorts implements instances.InstanceFirewaller.
func (inst *sdkInstance) ClosePorts(ctx envcontext.ProviderCallContext, machineId string, ports firewall.IngressRules) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.closePortsInGroup(ctx, name, ports); err != nil {
		return err
	}
	logger.Infof(ctx, "closed ports in security group %s: %v", name, ports)
	return nil
}

// IngressRules implements instances.InstanceFirewaller.
func (inst *sdkInstance) IngressRules(ctx envcontext.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
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

// FetchInstanceClient describes the funcs needed from the EC2 client for
// fetching instance types in a region. It's assumed that the ec2 client
// conforming to this interface is scoped to the region that instances are being
// requested for.
type FetchInstanceClient interface {
	// DescribeInstanceTypes is the same func as that of the ec2 client. See:
	// https://github.com/aws/aws-sdk-go-v2/blob/service/ec2/v1.123.0/service/ec2/api_op_DescribeInstanceTypes.go#L21
	DescribeInstanceTypes(context.Context, *ec2.DescribeInstanceTypesInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)
}

// FetchInstanceTypeInfo is responsible for fetching all of the
// available instance types for an AWS region. This func assumes that the ec2
// client provided is scoped to a region already.
func FetchInstanceTypeInfo(
	ctx context.Context,
	ec2Client FetchInstanceClient,
) ([]types.InstanceTypeInfo, error) {
	const maxResults = int32(100)

	instanceTypes := []types.InstanceTypeInfo{}
	var nextToken *string
	for {
		instTypeResults, err := ec2Client.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
			MaxResults: aws.Int32(maxResults),
			NextToken:  nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describing instance types: %w", err)
		}
		instanceTypes = append(instanceTypes, instTypeResults.InstanceTypes...)
		nextToken = instTypeResults.NextToken

		if nextToken == nil {
			break
		}
	}

	return instanceTypes, nil
}
