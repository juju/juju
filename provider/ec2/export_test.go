// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"gopkg.in/amz.v3/ec2"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	jujustorage "github.com/juju/juju/storage"
)

type EC2Client = ec2Client

func StorageEC2(vs jujustorage.VolumeSource) *ec2.EC2 {
	return vs.(*ebsVolumeSource).env.ec2
}

func JujuGroupName(e environs.Environ) string {
	return e.(*environ).jujuGroupName()
}

func MachineGroupName(e environs.Environ, machineId string) string {
	return e.(*environ).machineGroupName(machineId)
}

func EnvironEC2(e environs.Environ) *ec2.EC2 {
	return e.(*environ).ec2
}

func InstanceEC2(inst instances.Instance) *ec2.Instance {
	return inst.(*ec2Instance).Instance
}

func TerminatedInstances(e environs.Environ) ([]instances.Instance, error) {
	return e.(*environ).allInstancesByState(context.NewCloudCallContext(), "shutting-down", "terminated")
}

func InstanceSecurityGroups(e environs.Environ, ctx context.ProviderCallContext, ids []instance.Id, states ...string) ([]ec2.SecurityGroup, error) {
	return e.(*environ).instanceSecurityGroups(ctx, ids, states...)
}

func AllModelVolumes(e environs.Environ, ctx context.ProviderCallContext) ([]string, error) {
	return e.(*environ).allModelVolumes(ctx, true)
}

func AllModelGroups(e environs.Environ, ctx context.ProviderCallContext) ([]string, error) {
	return e.(*environ).modelSecurityGroupIDs(ctx)
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
