// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	amzec2 "gopkg.in/amz.v3/ec2"
)

type mockEC2Session struct {
	newInstancesClient func() *amzec2.EC2
}

func (*mockEC2Session) DescribeAvailabilityZones(*ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return &ec2.DescribeAvailabilityZonesOutput{
		AvailabilityZones: []*ec2.AvailabilityZone{{
			ZoneName: aws.String("test-available"),
		}},
	}, nil
}

func (s *mockEC2Session) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	// Proxy the DescribeInstances request through to the equivalent amz
	// package's Instances() method, as amz is still used to start the
	// instances.
	var ids []string
	for _, id := range input.InstanceIds {
		if id == nil {
			continue
		}
		ids = append(ids, *id)
	}

	filter := amzec2.NewFilter()
	for _, f := range input.Filters {
		if f.Name == nil {
			continue
		}
		var values []string
		for _, v := range f.Values {
			if v != nil {
				values = append(values, *v)
			}
		}
		filter.Add(*f.Name, values...)
	}

	client := s.newInstancesClient()
	resp, err := client.Instances(ids, filter)
	if err != nil {
		return nil, err
	}

	output := &ec2.DescribeInstancesOutput{}
	for _, r := range resp.Reservations {
		res := &ec2.Reservation{}
		for _, i := range r.Instances {
			inst := &ec2.Instance{
				InstanceId:   &i.InstanceId,
				InstanceType: &i.InstanceType,
				State: &ec2.InstanceState{
					Name: &i.State.Name,
				},
			}
			if i.PrivateIPAddress != "" {
				inst.PrivateIpAddress = &i.PrivateIPAddress
			}
			if i.IPAddress != "" {
				inst.PublicIpAddress = &i.IPAddress
			}
			for _, t := range i.Tags {
				t := t // make a copy so the address is new each loop
				inst.Tags = append(inst.Tags, &ec2.Tag{Key: &t.Key, Value: &t.Value})
			}
			res.Instances = append(res.Instances, inst)
		}
		output.Reservations = append(output.Reservations, res)
	}
	return output, nil
}

func (*mockEC2Session) DescribeInstanceTypeOfferings(*ec2.DescribeInstanceTypeOfferingsInput) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
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

func (*mockEC2Session) DescribeInstanceTypes(*ec2.DescribeInstanceTypesInput) (*ec2.DescribeInstanceTypesOutput, error) {
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

func (*mockEC2Session) DescribeSpotPriceHistory(*ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error) {
	return &ec2.DescribeSpotPriceHistoryOutput{
		SpotPriceHistory: nil,
	}, nil
}
