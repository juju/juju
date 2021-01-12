// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type mockEC2Session struct{}

func (mockEC2Session) DescribeAvailabilityZones(*ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return &ec2.DescribeAvailabilityZonesOutput{
		AvailabilityZones: []*ec2.AvailabilityZone{{
			ZoneName: aws.String("test-available"),
		}},
	}, nil
}

func (mockEC2Session) DescribeInstances(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	// TODO(benhoyt) - mock properly
	return &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			{
				Instances: []*ec2.Instance{
					{
						InstanceId:       aws.String("1234"),
						PrivateIpAddress: aws.String("1.2.3.4"),
						PublicIpAddress:  aws.String("10.0.0.1"),
						State: &ec2.InstanceState{
							Name: aws.String("running"),
						},
					},
				},
			},
		},
	}, nil
}

func (mockEC2Session) DescribeInstanceTypeOfferings(*ec2.DescribeInstanceTypeOfferingsInput) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
	return &ec2.DescribeInstanceTypeOfferingsOutput{
		InstanceTypeOfferings: []*ec2.InstanceTypeOffering{{
			InstanceType: aws.String("t3a.micro"),
			Location:     aws.String("test-available"),
			LocationType: aws.String("availability-zone"),
		}, {
			InstanceType: aws.String("t3a.medium"),
			Location:     aws.String("test-available"),
			LocationType: aws.String("availability-zone"),
		}, {
			InstanceType: aws.String("t2.medium"),
			Location:     aws.String("test-available"),
			LocationType: aws.String("availability-zone"),
		}, {
			InstanceType: aws.String("m1.small"),
			Location:     aws.String("test-available"),
			LocationType: aws.String("availability-zone"),
		}},
	}, nil
}

func (mockEC2Session) DescribeInstanceTypes(*ec2.DescribeInstanceTypesInput) (*ec2.DescribeInstanceTypesOutput, error) {
	return &ec2.DescribeInstanceTypesOutput{
		InstanceTypes: []*ec2.InstanceTypeInfo{{
			InstanceType: aws.String("t3a.micro"),
			VCpuInfo:     &ec2.VCpuInfo{DefaultVCpus: aws.Int64(2)},
			MemoryInfo:   &ec2.MemoryInfo{SizeInMiB: aws.Int64(1024)},
			ProcessorInfo: &ec2.ProcessorInfo{
				SupportedArchitectures:   []*string{aws.String("x86_64")},
				SustainedClockSpeedInGhz: aws.Float64(2.5),
			},
			CurrentGeneration: aws.Bool(true),
		}, {
			InstanceType: aws.String("t3a.medium"),
			VCpuInfo:     &ec2.VCpuInfo{DefaultVCpus: aws.Int64(2)},
			MemoryInfo:   &ec2.MemoryInfo{SizeInMiB: aws.Int64(4096)},
			ProcessorInfo: &ec2.ProcessorInfo{
				SupportedArchitectures:   []*string{aws.String("x86_64")},
				SustainedClockSpeedInGhz: aws.Float64(2.5),
			},
			CurrentGeneration: aws.Bool(true),
		}, {
			InstanceType: aws.String("t2.medium"),
			VCpuInfo:     &ec2.VCpuInfo{DefaultVCpus: aws.Int64(2)},
			MemoryInfo:   &ec2.MemoryInfo{SizeInMiB: aws.Int64(1024)},
			ProcessorInfo: &ec2.ProcessorInfo{
				SupportedArchitectures:   []*string{aws.String("x86_64")},
				SustainedClockSpeedInGhz: aws.Float64(2.5),
			},
			CurrentGeneration: aws.Bool(true),
		}, {
			InstanceType:      aws.String("m1.small"),
			MemoryInfo:        &ec2.MemoryInfo{SizeInMiB: aws.Int64(1024)},
			ProcessorInfo:     &ec2.ProcessorInfo{SupportedArchitectures: []*string{aws.String("x86_64")}},
			CurrentGeneration: aws.Bool(true),
		}},
	}, nil
}

func (mockEC2Session) DescribeSpotPriceHistory(*ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	return &ec2.DescribeSpotPriceHistoryOutput{
		SpotPriceHistory: nil,
	}, nil
}
