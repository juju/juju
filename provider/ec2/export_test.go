// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/aws/aws-sdk-go/service/ec2"
	amzec2 "gopkg.in/amz.v3/ec2"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	jujustorage "github.com/juju/juju/storage"
)

func StorageEC2(vs jujustorage.VolumeSource) *amzec2.EC2 {
	return vs.(*ebsVolumeSource).env.ec2
}

func JujuGroupName(e environs.Environ) string {
	return e.(*environ).jujuGroupName()
}

func MachineGroupName(e environs.Environ, machineId string) string {
	return e.(*environ).machineGroupName(machineId)
}

func EnvironEC2(e environs.Environ) *amzec2.EC2 {
	return e.(*environ).ec2
}

func InstanceEC2(inst instances.Instance) *amzec2.Instance {
	return inst.(*amzInstance).Instance
}

func InstanceSDKEC2(inst instances.Instance) *ec2.Instance {
	return inst.(*sdkInstance).i
}

func TerminatedInstances(e environs.Environ) ([]instances.Instance, error) {
	return e.(*environ).allInstancesByState(context.NewEmptyCloudCallContext(), "shutting-down", "terminated")
}

func InstanceSecurityGroups(e environs.Environ, ctx context.ProviderCallContext, ids []instance.Id, states ...string) ([]amzec2.SecurityGroup, error) {
	return e.(*environ).instanceSecurityGroups(ctx, ids, states...)
}

func AllModelVolumes(e environs.Environ, ctx context.ProviderCallContext) ([]string, error) {
	return e.(*environ).allModelVolumes(ctx, true)
}

func AllModelGroups(e environs.Environ, ctx context.ProviderCallContext) ([]string, error) {
	groups, err := e.(*environ).modelSecurityGroups(ctx)
	if err != nil {
		return nil, err
	}
	groupIds := make([]string, len(groups))
	for i, g := range groups {
		groupIds[i] = g.Id
	}
	return groupIds, nil
}

var (
	EC2AvailabilityZones           = &ec2AvailabilityZones
	RunInstances                   = &runInstances
	BlockDeviceNamer               = blockDeviceNamer
	GetBlockDeviceMappings         = getBlockDeviceMappings
	IsVPCNotUsableError            = isVPCNotUsableError
	IsVPCNotRecommendedError       = isVPCNotRecommendedError
	ShortAttempt                   = &shortAttempt
	DestroyVolumeAttempt           = &destroyVolumeAttempt
	DeleteSecurityGroupInsistently = &deleteSecurityGroupInsistently
	TerminateInstancesById         = &terminateInstancesById
	MaybeConvertCredentialError    = maybeConvertCredentialError
)

const VPCIDNone = vpcIDNone

func VerifyCredentials(env environs.Environ, ctx context.ProviderCallContext) error {
	return verifyCredentials(env.(*environ), ctx)
}
