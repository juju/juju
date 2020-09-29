// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils/v2/arch"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

var _ environs.InstanceTypesFetcher = (*environ)(nil)

type awsLogger struct {
	session *session.Session
}

func (l awsLogger) Log(args ...interface{}) {
	logger.Tracef("awsLogger %p: %s", l.session, fmt.Sprint(args...))
}

// EC2Session returns a session with the given credentials.
var EC2Session = func(region, accessKey, secretKey string) ec2iface.EC2API {
	sess := session.Must(session.NewSession())
	config := &aws.Config{
		Retryer: client.DefaultRetryer{ // these roughly match retry params in gopkg.in/amz.v3/ec2/ec2.go:EC2.query
			NumMaxRetries:    10,
			MinRetryDelay:    time.Second,
			MinThrottleDelay: time.Second,
			MaxRetryDelay:    time.Minute,
			MaxThrottleDelay: time.Minute,
		},
		Region: aws.String(region),
		Credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     accessKey,
			SecretAccessKey: secretKey,
		}),
	}

	// Enable request and response logging, but only if TRACE is enabled (as
	// they're probably fairly expensive to produce).
	if logger.IsTraceEnabled() {
		config.Logger = awsLogger{sess}
		config.LogLevel = aws.LogLevel(aws.LogDebug | aws.LogDebugWithRequestErrors | aws.LogDebugWithRequestRetries)
	}

	ec2Session := ec2.New(sess, config)
	return ec2Session
}

// InstanceTypes implements InstanceTypesFetcher
func (e *environ) InstanceTypes(ctx context.ProviderCallContext, c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	ec2Session := EC2Session(e.cloud.Region, e.ec2.AccessKey, e.ec2.SecretKey)
	iTypes, err := e.supportedInstanceTypes(ec2Session, ctx)
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
func virtType(info *ec2.InstanceTypeInfo) *string {
	// Only very old instance types are restricted to paravirtual.
	switch strings.SplitN(*info.InstanceType, ".", 2)[0] {
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

var archNames = map[string]string{
	"x86":     arch.I386,
	"x86_64":  arch.AMD64,
	"arm":     arch.ARM,
	"aarch64": arch.ARM64,
}

func archName(in string) string {
	if archName, ok := archNames[in]; ok {
		return archName
	}
	return in
}

func (e *environ) supportedInstanceTypes(ec2Session ec2iface.EC2API, ctx context.ProviderCallContext) (result []instances.InstanceType, err error) {
	e.instTypesMutex.Lock()
	defer e.instTypesMutex.Unlock()

	defer func() {
		if err == nil {
			if len(e.instTypes) == 0 {
				logger.Tracef("got instance types:\n%#v", result)
			}
			e.instTypes = result
		}
	}()

	// Use a cached copy if populated as it's mildly
	// expensive to fetch each time.
	// TODO(wallyworld) - consider using a cache with expiry
	if len(e.instTypes) > 0 {
		return e.instTypes, nil
	}

	const (
		maxOfferingsResults = 1000
		maxTypesPage        = 100
	)

	// First get all the zone names for the current region.
	zoneResults, err := ec2Session.DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{
		Filters: []*ec2.Filter{{
			Name:   aws.String("region-name"),
			Values: []*string{aws.String(e.cloud.Region)},
		}},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	var zoneNames []string
	zoneFilter := &ec2.Filter{Name: aws.String("location")}
	for _, z := range zoneResults.AvailabilityZones {
		// Should never be nil.
		if z.ZoneName == nil {
			continue
		}
		zoneNames = append(zoneNames, *z.ZoneName)
		zoneFilter.Values = append(zoneFilter.Values, z.ZoneName)
	}

	// Query the instance type names for the region and credential in use.
	var instTypeNames []*string
	instanceTypeZones := make(map[string]set.Strings)
	var token *string
	for {
		offeringResults, err := ec2Session.DescribeInstanceTypeOfferings(&ec2.DescribeInstanceTypeOfferingsInput{
			LocationType: aws.String("availability-zone"),
			MaxResults:   aws.Int64(maxOfferingsResults),
			NextToken:    token,
			Filters:      []*ec2.Filter{zoneFilter},
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, offering := range offeringResults.InstanceTypeOfferings {
			// Should never be nil.
			if offering.InstanceType == nil {
				continue
			}
			if _, ok := instanceTypeZones[*offering.InstanceType]; !ok {
				instanceTypeZones[*offering.InstanceType] = set.NewStrings()
			}
			instanceTypeZones[*offering.InstanceType].Add(*offering.Location)
			instTypeNames = append(instTypeNames, offering.InstanceType)
		}
		token = offeringResults.NextToken
		if token == nil {
			break
		}
	}

	// Populate the costs for the instance types in use.
	costs, err := instanceTypeCosts(ec2Session, instTypeNames, zoneNames)
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
		instTypeResults, err := ec2Session.DescribeInstanceTypes(instTypeParams)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, info := range instTypeResults.InstanceTypes {
			// Should never be nil.
			if info.InstanceType == nil {
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
	info *ec2.InstanceTypeInfo,
	instanceTypeZones map[string]set.Strings,
	costs map[string]uint64,
	zoneNames []string,
) instances.InstanceType {
	instCost, ok := costs[*info.InstanceType]
	if !ok {
		instCost = math.MaxUint64
	}

	instType := instances.InstanceType{
		Name:       *info.InstanceType,
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
			// Should never be nil.
			if instArch == nil {
				continue
			}
			instType.Arches = append(instType.Arches, archName(*instArch))
		}
	}
	instZones, ok := instanceTypeZones[instType.Name]
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
func instanceTypeCosts(ec2Session ec2iface.EC2API, instTypeNames []*string, zoneNames []string) (map[string]uint64, error) {
	const (
		maxResults = 1000
		costFactor = 1000
	)
	var token *string
	spParams := &ec2.DescribeSpotPriceHistoryInput{
		InstanceTypes: instTypeNames,
		MaxResults:    aws.Int64(maxResults),
		StartTime:     aws.Time(time.Now()),
		Filters: []*ec2.Filter{
			// Only look at Linux results (to reduce total number of results;
			// it's only an estimate anyway)
			{
				Name: aws.String("product-description"),
				Values: []*string{
					aws.String("Linux/UNIX"),
					aws.String("Linux/UNIX (Amazon VPC)"),
				},
			},
		},
	}
	if len(zoneNames) > 0 {
		// Just return results for the first availability zone (others are
		// probably similar, and we don't need to be too accurate here)
		filter := &ec2.Filter{
			Name:   aws.String("availability-zone"),
			Values: []*string{aws.String(zoneNames[0])},
		}
		spParams.Filters = append(spParams.Filters, filter)
	}

	costs := make(map[string]uint64)
	for {
		spParams.NextToken = token
		priceResults, err := ec2Session.DescribeSpotPriceHistory(spParams)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, sp := range priceResults.SpotPriceHistory {
			if _, ok := costs[*sp.InstanceType]; !ok {
				price, err := strconv.ParseFloat(*sp.SpotPrice, 32)
				if err == nil {
					costs[*sp.InstanceType] = uint64(costFactor * price)
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
