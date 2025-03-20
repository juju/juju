// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
)

// instanceType is a strongly typed representation of an AWS instance type
// string. Useful for extracting the logic parts of the instance type string.
type instanceType struct {
	Capabilities    set.Strings
	Generation      int
	Family          string
	ProcessorFamily string
	Size            string
}

type instanceTypeCache struct {
	ec2Client            Client
	instTypesInfo        []types.InstanceTypeInfo
	instFamilyGeneration map[string]int
	populateMutex        sync.Mutex
	region               string
}

// instanceTypeFilter is an interface for declaring a capability to filter aws
// instances on.
type instanceTypeFilter interface {
	// Filter is given an AWS instance type and makes a decision if it should be
	// filtered out or in of the current decision pipeline. The decision is up
	// to filter. Returns true for keep and false for reject.
	Filter(types.InstanceTypeInfo) bool
}

type instanceTypeFilterFunc func(info types.InstanceTypeInfo) bool

const (
	processorFamilyIntel    = "i"
	processorFamilyAMD      = "a"
	processorFamilyGraviton = "g"

	// defaultAWSEC2Family is the default family class Juju prefers for using
	// when starting new AWS EC2 machines. In this case it's m as the best
	// general purpose machine type.
	defaultAWSEC2Family = "m"
)

var _ environs.InstanceTypesFetcher = (*environ)(nil)

// allInstanceTypeFilter takes a set of filters to run and will only return true
// for the filter condition if all filters passed. If no filters are supplied
// then true is returned.
func allInstanceTypeFilter(filters ...instanceTypeFilter) instanceTypeFilter {
	return instanceTypeFilterFunc(func(i types.InstanceTypeInfo) bool {
		for _, f := range filters {
			if !f.Filter(i) {
				return false
			}
		}
		return true
	})
}

// oneOfInstanceTypeFilter takes a set of filters to run and will return the
// first true response returned by a filter. If no filters evaluate to true then
// false is returned.
func oneOfInstanceTypeFilter(filters ...instanceTypeFilter) instanceTypeFilter {
	return instanceTypeFilterFunc(func(i types.InstanceTypeInfo) bool {
		for _, f := range filters {
			if f.Filter(i) {
				return true
			}
		}
		return false
	})
}

// currentGenInstanceTypeFilter filters out any instance type that is not
// current generation.
func currentGenInstanceTypeFilter() instanceTypeFilter {
	return instanceTypeFilterFunc(func(i types.InstanceTypeInfo) bool {
		return aws.ToBool(i.CurrentGeneration)
	})
}

// exactInstanceTypeFilter filters out instances that don't exactly match the
// instance type supplied.
func exactInstanceTypeFilter(match types.InstanceType) instanceTypeFilter {
	return instanceTypeFilterFunc(func(i types.InstanceTypeInfo) bool {
		return i.InstanceType == match
	})
}

// selectorInstanceTypeFilter will match on instances that contain the same
// values specified in the instance types. Zero values will be ignored.
func selectorInstanceTypeFilter(selector instanceType) instanceTypeFilter {
	return instanceTypeFilterFunc(func(i types.InstanceTypeInfo) bool {
		itDetails, err := parseInstanceType(i.InstanceType)
		if err != nil {
			return false
		}
		if !itDetails.Capabilities.Difference(selector.Capabilities).IsEmpty() {
			return false
		}
		if itDetails.Family != selector.Family && selector.Family != "" {
			return false
		}
		if itDetails.ProcessorFamily != selector.ProcessorFamily && selector.ProcessorFamily != "" {
			return false
		}
		if itDetails.Generation != selector.Generation && selector.Generation != 0 {
			return false
		}
		if itDetails.Size != selector.Size && selector.Size != "" {
			return false
		}

		return true
	})
}

// generalPurposeInstanceFilter supplies a filter that is capable of filtering
// only on machines that are considered general purpose enough for Juju to use
// as a sane default.
func (e *environ) generalPurposeInstanceFilter(
	ctx context.Context,
	cache *instanceTypeCache,
) (instanceTypeFilter, error) {
	highestGenerationIntel, err := cache.HighestFamilyGeneration(
		ctx,
		defaultAWSEC2Family,
		processorFamilyIntel,
	)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, err
	}

	highestGenerationGraviton, err := cache.HighestFamilyGeneration(
		ctx,
		defaultAWSEC2Family,
		processorFamilyGraviton,
	)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, err
	}

	return allInstanceTypeFilter(
		currentGenInstanceTypeFilter(),
		oneOfInstanceTypeFilter(
			selectorInstanceTypeFilter(instanceType{
				Family:          defaultAWSEC2Family,
				Generation:      highestGenerationIntel,
				ProcessorFamily: processorFamilyIntel,
			}),
			selectorInstanceTypeFilter(instanceType{
				Family:          defaultAWSEC2Family,
				Generation:      highestGenerationGraviton,
				ProcessorFamily: processorFamilyGraviton,
			}),
		),
	), nil
}

// Filter implements instanceFilter Filter.
func (f instanceTypeFilterFunc) Filter(i types.InstanceTypeInfo) bool {
	return f(i)
}

// filterInstanceTypes is a helper function to a run a filter over a set of
func filterInstanceTypes(instanceTypes []types.InstanceTypeInfo, filter instanceTypeFilter) []types.InstanceTypeInfo {
	filtered := []types.InstanceTypeInfo{}
	for _, instanceType := range instanceTypes {
		if filter.Filter(instanceType) {
			filtered = append(filtered, instanceType)
		}
	}
	return filtered
}

func parseInstanceType(instType types.InstanceType) (instanceType, error) {
	parts := strings.Split(string(instType), ".")
	if len(parts) != 2 {
		return instanceType{}, fmt.Errorf("unknown instance type %q, expected . separator", instType)
	}

	rval := instanceType{
		Capabilities: set.NewStrings(),
		Size:         parts[1],
	}

	var (
		family       string
		generation   string
		capabilities string
	)
	placement := &family
	for _, r := range parts[0] {
		if unicode.IsDigit(r) {
			placement = &capabilities
			generation += string(r)
			continue
		}
		*placement += string(r)
	}
	rval.Family = family

	genInt, err := strconv.Atoi(generation)
	if err != nil {
		return rval, fmt.Errorf("failed to parse generation number from type %q: %w", instType, err)
	}
	rval.Generation = genInt

	capSplits := strings.Split(capabilities, "-")
	if len(capSplits) == 0 {
		capSplits = []string{""}
	}

	for _, r := range capSplits[0] {
		switch string(r) {
		case processorFamilyAMD,
			processorFamilyIntel,
			processorFamilyGraviton:
			rval.ProcessorFamily = string(r)
		default:
			rval.Capabilities.Add(string(r))
		}
	}

	// If the processor family doesn't get set it's one of AWS's older instance
	// types and so we can assume intel.
	if rval.ProcessorFamily == "" {
		rval.ProcessorFamily = processorFamilyIntel
	}

	capSplits = capSplits[1:]
	for _, split := range capSplits {
		rval.Capabilities.Add(split)
	}

	return rval, nil
}

// populateCache loads aws instance type info the region into the cache
func (c *instanceTypeCache) populateCache(ctx context.Context) error {
	c.populateMutex.Lock()
	defer c.populateMutex.Unlock()

	if c.instTypesInfo != nil && c.instFamilyGeneration != nil {
		return nil
	}

	instTypesInfo, err := FetchInstanceTypeInfo(
		ctx,
		c.ec2Client,
	)
	if err != nil {
		return fmt.Errorf("populating instance type cache for region %q: %w", c.region, err)
	}

	c.instTypesInfo = instTypesInfo
	c.instFamilyGeneration = highestFamilyProcessorGeneration(c.instTypesInfo)
	return nil
}

// newInstanceTypeCache constructs a new instance type cache for the specified
// region.
func newInstanceTypeCache(ec2Client Client, region string) *instanceTypeCache {
	return &instanceTypeCache{
		ec2Client: ec2Client,
		region:    region,
	}
}

// HighestFamilyGeneration returns the highest generation supported for the
// provided aws ec2 family. If no generation is found for the family then an
// error satisfying NotFound is returned.
func (c *instanceTypeCache) HighestFamilyGeneration(
	ctx context.Context,
	family,
	processor string,
) (int, error) {
	if err := c.populateCache(ctx); err != nil {
		return 0, err
	}

	gen, exists := c.instFamilyGeneration[family+processor]
	if !exists {
		return 0, fmt.Errorf("%w generation for family %q", errors.NotFound, family)
	}

	return gen, nil
}

// InstanceTypesInfo returns the cached instance type info or an error if there
// was a problem loading the cache.
func (c *instanceTypeCache) InstanceTypesInfo(
	ctx context.Context,
) ([]types.InstanceTypeInfo, error) {
	if err := c.populateCache(ctx); err != nil {
		return nil, err
	}
	return c.instTypesInfo, nil
}

// InstanceTypes implements InstanceTypesFetcher
func (e *environ) InstanceTypes(ctx context.Context, c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	iTypeFilter := allInstanceTypeFilter()
	iTypes, err := e.supportedInstanceTypes(ctx, iTypeFilter)
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(e.HandleCredentialError(ctx, err))
	}
	iTypes, err = instances.MatchingInstanceTypes(iTypes, "", c)
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	return instances.InstanceTypesWithCostMetadata{
		InstanceTypes: iTypes,
		CostUnit:      "",
		CostCurrency:  "USD",
	}, nil
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

var archNames = map[types.ArchitectureType]string{
	types.ArchitectureTypeX8664: arch.AMD64,
	types.ArchitectureTypeArm64: arch.ARM64,
}

func archName(in types.ArchitectureType) string {
	if archName, ok := archNames[in]; ok {
		return archName
	}
	return string(in)
}

func (e *environ) supportedInstanceTypes(
	ctx context.Context,
	filter instanceTypeFilter,
) ([]instances.InstanceType, error) {
	instanceTypes, err := e.instanceTypeCache().InstanceTypesInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("consulting cache for available instance types: %w", err)
	}

	instTypes := filterInstanceTypes(instanceTypes, filter)
	return convertEC2InstanceTypes(instTypes), nil
}

// convertEC2InstanceTypes will convert a slice of AWS InstanceTypeInfo to a
// Juju InstanceType slice.
func convertEC2InstanceTypes(instTypes []types.InstanceTypeInfo) []instances.InstanceType {
	rval := make([]instances.InstanceType, 0, len(instTypes))
	for _, instType := range instTypes {
		rval = append(rval, convertEC2InstanceType(instType))
	}
	return rval
}

// convertEC2InstanceType will convert an AWS InstanceTypeInfo to a Juju
// InstanceType.
func convertEC2InstanceType(
	info types.InstanceTypeInfo,
) instances.InstanceType {
	instType := instances.InstanceType{
		Name:     string(info.InstanceType),
		VirtType: virtType(info),
		Networking: instances.InstanceTypeNetworking{
			SupportsIPv6: info.NetworkInfo != nil && aws.ToBool(info.NetworkInfo.Ipv6Supported),
		},
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
		unsupportedSet := set.NewStrings(arch.UnsupportedArches...)
		for _, instArch := range info.ProcessorInfo.SupportedArchitectures {
			// Should never be empty.
			if instArch == "" {
				continue
			}
			// Ensure that we're not attempting to use an unsupported
			// architecture (i386).
			if unsupportedSet.Contains(string(instArch)) {
				continue
			}
			instType.Arch = archName(instArch)
			break
		}
	}

	return instType
}

// highestFamilyProcessorGeneration takes a slice of InstancceTypeInfo structs
// and  calculates the highest generation supported by each family and processor
// family. This is useful for Juju to align it's use of families on to the
// latest generation hardware.
func highestFamilyProcessorGeneration(instances []types.InstanceTypeInfo) map[string]int {
	rval := map[string]int{}
	for _, instance := range instances {
		it, err := parseInstanceType(instance.InstanceType)
		if err != nil {
			continue
		}

		if rval[it.Family+it.ProcessorFamily] < it.Generation {
			rval[it.Family+it.ProcessorFamily] = it.Generation
		}
	}
	return rval
}
