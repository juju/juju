// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils/v2/arch"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

var _ environs.InstanceTypesFetcher = (*environ)(nil)

// InstanceTypes implements InstanceTypesFetcher
func (e *environ) InstanceTypes(ctx context.ProviderCallContext, c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	iTypes, err := e.supportedInstanceTypes(ctx)
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	iTypes, err = instances.MatchingInstanceTypes(iTypes, "", c)
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	return instances.InstanceTypesWithCostMetadata{
		InstanceTypes: iTypes,
		CostUnit:      "$USD/hour",
		CostDivisor:   1000,
		CostCurrency:  "USD"}, nil
}

func calculateCPUPower(instType string, clock *float64, vcpu uint64) uint64 {
	// T-class instances have burstable CPU. This is not captured
	// in the pricing information, so we have to hard-code it. We
	// will have to update this list when T3 instances come along.
	switch instType {
	case "t1.micro":
		return 20
	case "t2.nano":
		return 5
	case "t2.micro":
		return 10
	case "t2.small":
		return 20
	case "t2.medium":
		return 40
	case "t2.large":
		return 60
	}
	if clock == nil {
		return vcpu * 100
	}

	// If the information includes a clock speed, we use that
	// to estimate the ECUs. The pricing information does not
	// include the ECUs, but they're only estimates anyway.
	// Amazon moved to "vCPUs" quite some time ago.
	// To date, info.ClockSpeed can have the form "Up to <float> GHz" or
	// "<float> GHz", so look for a float match.
	return uint64(*clock * 1.4 * 100 * float64(vcpu))
}

// virtType returns a virt type that matches our simplestrams metadata
// rather than what the instance type actually reports.
func virtType(info types.InstanceTypeInfo) *string {
	// Only very old instance types are restricted to paravirtual.
	switch strings.SplitN(string(info.InstanceType), ".", 2)[0] {
	case "t1", "m1", "c1", "m2":
		return aws.String("paravirtual")
	}

	return aws.String("hvm")
}

// supportsClassic reports whether the instance type with the given
// name can be run in EC2-Classic.
//
// At the time of writing, we know that the following instance type
// families support only VPC: C4, M4, P2, T2, X1. However, rather
// than hard-coding that list, we assume that any new instance type
// families support VPC only, and so we hard-code the inverse of the
// list at the time of writing.
//
// See:
//     http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-vpc.html#vpc-only-instance-types
func supportsClassic(instanceType string) bool {
	classicTypes := set.NewStrings(
		"c1", "c3",
		"cc2",
		"cg1",
		"cr1",
		"d2",
		"g2",
		"hi1",
		"hs1",
		"i2",
		"m1", "m2", "m3",
		"r3",
		"t1",
	)
	parts := strings.SplitN(instanceType, ".", 2)
	if len(parts) < 2 {
		return false
	}
	return classicTypes.Contains(strings.ToLower(parts[0]))
}

var archNames = map[types.ArchitectureType]string{
	"x86":     arch.I386,
	"x86_64":  arch.AMD64,
	"arm":     arch.ARM,
	"aarch64": arch.ARM64,
}

func archName(in types.ArchitectureType) string {
	if archName, ok := archNames[in]; ok {
		return archName
	}
	return string(in)
}

func (e *environ) supportedInstanceTypes(ctx context.ProviderCallContext) ([]instances.InstanceType, error) {
	e.instTypesMutex.Lock()
	defer e.instTypesMutex.Unlock()

	// Use a cached copy if populated as it's mildly
	// expensive to fetch each time.
	// TODO(wallyworld) - consider using a cache with expiry
	if len(e.instTypes) == 0 {
		instTypes, err := e.collectSupportedInstanceTypes(ctx)
		if err != nil {
			return nil, err
		}
		e.instTypes = instTypes
	}

	return e.instTypes, nil
}

// collectSupportedInstanceTypes queries several EC2 APIs and combines the
// results into a slice of InstanceType values.
//
// This method must be called while holding the instTypesMutex.
func (e *environ) collectSupportedInstanceTypes(ctx context.ProviderCallContext) ([]instances.InstanceType, error) {
	const (
		maxOfferingsResults = 1000
		maxTypesPage        = 100
	)

	// First get all the zone names for the current region.
	zoneResults, err := e.ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []types.Filter{makeFilter("region-name", e.cloud.Region)},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	var zoneNames []string
	zoneFilter := types.Filter{Name: aws.String("location")}
	for _, z := range zoneResults.AvailabilityZones {
		// Should never be nil.
		if z.ZoneName == nil {
			continue
		}
		zoneNames = append(zoneNames, *z.ZoneName)
		zoneFilter.Values = append(zoneFilter.Values, aws.ToString(z.ZoneName))
	}

	// Query the instance type names for the region and credential in use.
	var instTypeNames []types.InstanceType
	instanceTypeZones := make(map[types.InstanceType]set.Strings)
	var token *string
	for {
		offeringResults, err := e.ec2Client.DescribeInstanceTypeOfferings(ctx, &ec2.DescribeInstanceTypeOfferingsInput{
			LocationType: "availability-zone",
			MaxResults:   aws.Int32(maxOfferingsResults),
			NextToken:    token,
			Filters:      []types.Filter{zoneFilter},
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, offering := range offeringResults.InstanceTypeOfferings {
			// Should never be empty.
			if offering.InstanceType == "" {
				continue
			}
			if _, ok := instanceTypeZones[offering.InstanceType]; !ok {
				instanceTypeZones[offering.InstanceType] = set.NewStrings()
			}
			instanceTypeZones[offering.InstanceType].Add(*offering.Location)
			instTypeNames = append(instTypeNames, offering.InstanceType)
		}
		token = offeringResults.NextToken
		if token == nil {
			break
		}
	}

	// Populate the costs for the instance types in use.
	costs, err := instanceTypeCosts(e.ec2Client, ctx, instTypeNames, zoneNames)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Compose the results.
	var allInstanceTypes []instances.InstanceType
	for len(instTypeNames) > 0 {
		querySize := len(instTypeNames)
		if querySize > maxTypesPage {
			querySize = maxTypesPage
		}
		page := instTypeNames[0:querySize]
		instTypeNames = instTypeNames[querySize:]

		instTypeParams := &ec2.DescribeInstanceTypesInput{
			InstanceTypes: page,
		}
		instTypeResults, err := e.ec2Client.DescribeInstanceTypes(ctx, instTypeParams)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, info := range instTypeResults.InstanceTypes {
			// Should never be empty.
			if info.InstanceType == "" {
				continue
			}
			allInstanceTypes = append(
				allInstanceTypes, convertEC2InstanceType(info, instanceTypeZones, costs, zoneNames))
		}
	}

	if isVPCIDSet(e.ecfg().vpcID()) {
		return allInstanceTypes, nil
	}
	hasDefaultVPC, err := e.hasDefaultVPC(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if hasDefaultVPC {
		return allInstanceTypes, nil
	}

	// The region has no default VPC, and the user has not specified
	// one to use. We filter out any instance types that are not
	// supported in EC2-Classic.
	var supportedInstanceTypes []instances.InstanceType
	for _, instanceType := range allInstanceTypes {
		if !supportsClassic(instanceType.Name) {
			continue
		}
		supportedInstanceTypes = append(supportedInstanceTypes, instanceType)
	}
	return supportedInstanceTypes, nil
}

func convertEC2InstanceType(
	info types.InstanceTypeInfo,
	instanceTypeZones map[types.InstanceType]set.Strings,
	costs map[types.InstanceType]uint64,
	zoneNames []string,
) instances.InstanceType {
	instCost, ok := costs[info.InstanceType]
	if !ok {
		instCost = math.MaxUint64
	}

	instType := instances.InstanceType{
		Name:       string(info.InstanceType),
		VirtType:   virtType(info),
		Deprecated: info.CurrentGeneration == nil || !*info.CurrentGeneration,
		Cost:       instCost,
	}
	if info.VCpuInfo != nil && info.VCpuInfo.DefaultVCpus != nil {
		instType.CpuCores = uint64(*info.VCpuInfo.DefaultVCpus)
		if info.ProcessorInfo != nil && info.ProcessorInfo.SustainedClockSpeedInGhz != nil {
			cpupower := calculateCPUPower(
				instType.Name,
				info.ProcessorInfo.SustainedClockSpeedInGhz,
				uint64(*info.VCpuInfo.DefaultVCpus))
			instType.CpuPower = &cpupower
		}
	}
	if info.MemoryInfo != nil && info.MemoryInfo.SizeInMiB != nil {
		instType.Mem = uint64(*info.MemoryInfo.SizeInMiB)
	}
	if info.ProcessorInfo != nil {
		for _, instArch := range info.ProcessorInfo.SupportedArchitectures {
			// Should never be empty.
			if instArch == "" {
				continue
			}
			instType.Arches = append(instType.Arches, archName(instArch))
		}
	}
	instZones, ok := instanceTypeZones[types.InstanceType(instType.Name)]
	if !ok {
		instType.Deprecated = true
	} else {
		// If a instance type is available it at least 3 zones (or all of them if < 3)
		// then consider it able to be used without explicitly asking for it.
		instType.Deprecated = instZones.Size() < 3 && instZones.Size() < len(zoneNames)
	}
	return instType
}

// instanceTypeCosts queries the latest spot price for the given instance types.
func instanceTypeCosts(ec2Client Client, ctx context.ProviderCallContext, instTypeNames []types.InstanceType, zoneNames []string) (map[types.InstanceType]uint64, error) {
	const (
		maxResults = 1000
		costFactor = 1000
	)
	var token *string
	spParams := &ec2.DescribeSpotPriceHistoryInput{
		InstanceTypes: instTypeNames,
		MaxResults:    aws.Int32(maxResults),
		StartTime:     aws.Time(time.Now()),
		// Only look at Linux results (to reduce total number of results;
		// it's only an estimate anyway)
		Filters: []types.Filter{makeFilter("product-description", "Linux/UNIX", "Linux/UNIX (Amazon VPC)")},
	}
	if len(zoneNames) > 0 {
		// Just return results for the first availability zone (others are
		// probably similar, and we don't need to be too accurate here)
		filter := makeFilter("availability-zone", zoneNames[0])
		spParams.Filters = append(spParams.Filters, filter)
	}

	costs := make(map[types.InstanceType]uint64)
	for {
		spParams.NextToken = token
		priceResults, err := ec2Client.DescribeSpotPriceHistory(ctx, spParams)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, sp := range priceResults.SpotPriceHistory {
			if _, ok := costs[sp.InstanceType]; !ok {
				price, err := strconv.ParseFloat(*sp.SpotPrice, 32)
				if err == nil {
					costs[sp.InstanceType] = uint64(costFactor * price)
				}
			}
		}
		token = priceResults.NextToken
		// token should be nil when there's no more records
		// but it never gets set to nil so there's a bug in the api.
		if len(priceResults.SpotPriceHistory) < maxResults {
			break
		}
	}
	return costs, nil
}
