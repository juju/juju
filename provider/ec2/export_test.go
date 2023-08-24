// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	stdcontext "context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	jujustorage "github.com/juju/juju/internal/storage"
)

func StorageEC2(vs jujustorage.VolumeSource) Client {
	return vs.(*ebsVolumeSource).env.ec2Client
}

func JujuGroupName(e environs.Environ) string {
	return e.(*environ).jujuGroupName()
}

func MachineGroupName(e environs.Environ, machineId string) string {
	return e.(*environ).machineGroupName(machineId)
}

func EnvironEC2Client(e environs.Environ) Client {
	return e.(*environ).ec2Client
}

func InstanceSDKEC2(inst instances.Instance) types.Instance {
	return inst.(*sdkInstance).i
}

func TerminatedInstances(e environs.Environ) ([]instances.Instance, error) {
	return e.(*environ).allInstancesByState(context.NewEmptyCloudCallContext(), "shutting-down", "terminated")
}

func InstanceSecurityGroups(e environs.Environ, ctx context.ProviderCallContext, ids []instance.Id, states ...string) ([]types.GroupIdentifier, error) {
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
		groupIds[i] = aws.ToString(g.GroupId)
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
	ShortRetryStrategy             = &shortRetryStrategy
	DestroyVolumeAttempt           = &destroyVolumeAttempt
	DeleteSecurityGroupInsistently = &deleteSecurityGroupInsistently
	TerminateInstancesById         = &terminateInstancesById
	MaybeConvertCredentialError    = maybeConvertCredentialError
)

const VPCIDNone = vpcIDNone

type stubAccountAPIClient struct {
	Client
}

func (s *stubAccountAPIClient) DescribeAccountAttributes(
	_ stdcontext.Context, _ *ec2.DescribeAccountAttributesInput, _ ...func(*ec2.Options),
) (*ec2.DescribeAccountAttributesOutput, error) {
	return nil, errors.New("boom")
}

func VerifyCredentials(ctx context.ProviderCallContext) error {
	return verifyCredentials(&stubAccountAPIClient{}, ctx)
}
