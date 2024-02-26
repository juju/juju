// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// DescribeInstanceTypeOfferings implements ec2.Client.
func (*Server) DescribeInstanceTypeOfferings(_ context.Context, _ *ec2.DescribeInstanceTypeOfferingsInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
	result := &ec2.DescribeInstanceTypeOfferingsOutput{}
	for _, instTypeName := range []types.InstanceType{"t3a.micro", "t3a.medium", "t2.medium", "m1.small"} {
		for _, zoneName := range []string{"test-available", "test-impaired", "test-unavailable", "test-available2"} {
			result.InstanceTypeOfferings = append(result.InstanceTypeOfferings, types.InstanceTypeOffering{
				InstanceType: instTypeName,
				Location:     aws.String(zoneName),
				LocationType: "availability-zone",
			})
		}
	}
	return result, nil
}

// DescribeInstanceTypes implements ec2.Client.
func (*Server) DescribeInstanceTypes(_ context.Context, _ *ec2.DescribeInstanceTypesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error) {
	return &ec2.DescribeInstanceTypesOutput{
		InstanceTypes: []types.InstanceTypeInfo{{
			InstanceType: "t3a.micro",
			VCpuInfo:     &types.VCpuInfo{DefaultVCpus: aws.Int32(2)},
			MemoryInfo:   &types.MemoryInfo{SizeInMiB: aws.Int64(1024)},
			NetworkInfo: &types.NetworkInfo{
				Ipv6Supported: aws.Bool(true),
			},
			ProcessorInfo: &types.ProcessorInfo{
				SupportedArchitectures:   []types.ArchitectureType{"x86_64"},
				SustainedClockSpeedInGhz: aws.Float64(2.5),
			},
			CurrentGeneration: aws.Bool(true),
		}, {
			InstanceType: "t3a.medium",
			VCpuInfo:     &types.VCpuInfo{DefaultVCpus: aws.Int32(2)},
			MemoryInfo:   &types.MemoryInfo{SizeInMiB: aws.Int64(4096)},
			NetworkInfo: &types.NetworkInfo{
				Ipv6Supported: aws.Bool(true),
			},
			ProcessorInfo: &types.ProcessorInfo{
				SupportedArchitectures:   []types.ArchitectureType{"x86_64"},
				SustainedClockSpeedInGhz: aws.Float64(2.5),
			},
			CurrentGeneration: aws.Bool(true),
		}, {
			InstanceType: "t2.medium",
			VCpuInfo:     &types.VCpuInfo{DefaultVCpus: aws.Int32(2)},
			MemoryInfo:   &types.MemoryInfo{SizeInMiB: aws.Int64(1024)},
			NetworkInfo: &types.NetworkInfo{
				Ipv6Supported: aws.Bool(true),
			},
			ProcessorInfo: &types.ProcessorInfo{
				SupportedArchitectures:   []types.ArchitectureType{"x86_64"},
				SustainedClockSpeedInGhz: aws.Float64(2.5),
			},
			CurrentGeneration: aws.Bool(true),
		}, {
			InstanceType:      "m1.small",
			MemoryInfo:        &types.MemoryInfo{SizeInMiB: aws.Int64(1024)},
			ProcessorInfo:     &types.ProcessorInfo{SupportedArchitectures: []types.ArchitectureType{"x86_64"}},
			CurrentGeneration: aws.Bool(true),
		}, {
			InstanceType: "m6i.large",
			VCpuInfo:     &types.VCpuInfo{DefaultVCpus: aws.Int32(2)},
			MemoryInfo:   &types.MemoryInfo{SizeInMiB: aws.Int64(8192)},
			ProcessorInfo: &types.ProcessorInfo{
				SupportedArchitectures:   []types.ArchitectureType{"x86_64"},
				SustainedClockSpeedInGhz: aws.Float64(2.5),
			},
			CurrentGeneration: aws.Bool(true),
		}},
	}, nil
}

// DescribeSpotPriceHistory implements ec2.Client.
func (*Server) DescribeSpotPriceHistory(_ context.Context, _ *ec2.DescribeSpotPriceHistoryInput, _ ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	return &ec2.DescribeSpotPriceHistoryOutput{
		SpotPriceHistory: nil,
	}, nil
}
