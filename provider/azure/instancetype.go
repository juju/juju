// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"regexp"
	"strconv"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-11-01/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/utils/v3/arch"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/azure/internal/imageutils"
)

const defaultMem = 1024 // 1GiB

var instSizeVersionRegexp = regexp.MustCompile(`(?P<name>.*)_v(?P<version>\d\d?)?$`)

// The ordering here is for linux vm costs for the eastus region
// obtained from https://azureprice.net. This is used as a fallback
// where real costs are not available, to ensure we choose the right
// VMs given matching constraints. The assumption is that all regions
// have the same relative costs.
//
// xxxS is the same price as xxx, but is targeted at Premium Storage.
// We put the premium storage variants directly after their non-premium counterparts.
var machineSizeCost = []string{
	"Standard_A0",
	"Standard_A1",
	"Standard_F1",
	"Standard_F1s",
	"Standard_DS1",
	"Standard_D1",
	"Standard_D2as",
	"Standard_A2",
	"Standard_D2",
	"Standard_D2s",
	"Standard_D2a",
	"Standard_DC1s",
	"Standard_F2",
	"Standard_F2s",
	"Standard_D2ads",
	"Standard_E2as",
	"Standard_D2d",
	"Standard_D2ds",
	"Standard_A2m",
	"Standard_E2",
	"Standard_E2s",
	"Standard_E2a",
	"Standard_E2ads",
	"Standard_DC2as",
	"Standard_E2d",
	"Standard_E2ds",
	"Standard_DS2",
	"Standard_DC2ads",
	"Standard_EC2as",
	"Standard_D11",
	"Standard_DS11",
	"Standard_DS11-1",
	"Standard_A4",
	"Standard_D4",
	"Standard_D4a",
	"Standard_D4s",
	"Standard_DC2s",
	"Standard_D4as",
	"Standard_F4",
	"Standard_F4s",
	"Standard_D4ads",
	"Standard_EC2ads",
	"Standard_D4d",
	"Standard_D4ds",
	"Standard_NV4as",
	"Standard_A4m",
	"Standard_A3",
	"Standard_A5",
	"Standard_E4",
	"Standard_E4s",
	"Standard_E4as",
	"Standard_E4-2s",
	"Standard_E4a",
	"Standard_E4ads",
	"Standard_E4-2as",
	"Standard_E4-2ads",
	"Standard_DC4as",
	"Standard_E4-2ds",
	"Standard_E4d",
	"Standard_E4ds",
	"Standard_D3",
	"Standard_DS3",
	"Standard_DC4ads",
	"Standard_EC4as",
	"Standard_D12",
	"Standard_DS12",
	"Standard_DS12-1",
	"Standard_DS12-2",
	"Standard_FX4mds",
	"Standard_D8",
	"Standard_D8s",
	"Standard_D8a",
	"Standard_DC4s",
	"Standard_D8as",
	"Standard_F8",
	"Standard_F8s",
	"Standard_A8",
	"Standard_D8ads",
	"Standard_EC4ads",
	"Standard_D8d",
	"Standard_D8ds",
	"Standard_NV8as",
	"Standard_A8m",
	"Standard_A6",
	"Standard_E8",
	"Standard_E8s",
	"Standard_E8-2s",
	"Standard_E8-4s",
	"Standard_E8a",
	"Standard_E8-4as",
	"Standard_E8as",
	"Standard_E8-2as",
	"Standard_E8ads",
	"Standard_E8-2ads",
	"Standard_E8-4ads",
	"Standard_NC4as_T4",
	"Standard_DC8as",
	"Standard_E8d",
	"Standard_E8ds",
	"Standard_E8-4ds",
	"Standard_E8-2ds",
	"Standard_DS4",
	"Standard_L8s",
	"Standard_DC8ads",
	"Standard_EC8as",
	"Standard_DS13-4",
	"Standard_D13",
	"Standard_DS13-2",
	"Standard_NC8as_T4",
	"Standard_D16",
	"Standard_D16s",
	"Standard_D16as",
	"Standard_DC8",
	"Standard_D16a",
	"Standard_DS13",
	"Standard_F16",
	"Standard_F16s",
	"Standard_D16ads",
	"Standard_PB6s",
	"Standard_EC8ads",
	"Standard_NC6",
	"Standard_D16d",
	"Standard_D16ds",
	"Standard_H8",
	"Standard_NV16as",
	"Standard_A7",
	"Standard_E16",
	"Standard_E16s",
	"Standard_E16-4s",
	"Standard_E16-8s",
	"Standard_E16a",
	"Standard_E16-8as",
	"Standard_E16as",
	"Standard_E16-4as",
	"Standard_E16-4ads",
	"Standard_E16ads",
	"Standard_E16-8ads",
	"Standard_DC16as",
	"Standard_FX12mds",
	"Standard_NV6",
	"Standard_NV12s",
	"Standard_E16d",
	"Standard_E16ds",
	"Standard_E16-4ds",
	"Standard_E16-8ds",
	"Standard_DS5",
	"Standard_D5",
	"Standard_NC16as_T4",
	"Standard_H8m",
	"Standard_L16s",
	"Standard_E20",
	"Standard_E20s",
	"Standard_E20a",
	"Standard_E20as",
	"Standard_DC16ads",
	"Standard_E20ads",
	"Standard_F32s",
	"Standard_EC16as",
	"Standard_E20d",
	"Standard_E20ds",
	"Standard_D14",
	"Standard_DS14",
	"Standard_DS14-8",
	"Standard_DS14-4",
	"Standard_D32",
	"Standard_D32s",
	"Standard_D32as",
	"Standard_D32a",
	"Standard_M8-2ms",
	"Standard_M8-4ms",
	"Standard_M8ms",
	"Standard_D32ads",
	"Standard_NP10s",
	"Standard_EC16ads",
	"Standard_EC20as",
	"Standard_NC12",
	"Standard_H16",
	"Standard_D32d",
	"Standard_D32ds",
	"Standard_D15",
	"Standard_DS15",
	"Standard_NV32as",
	"Standard_H16r",
	"Standard_E32",
	"Standard_E32-8s",
	"Standard_E32-16s",
	"Standard_E32s",
	"Standard_E32-8as",
	"Standard_E32as",
	"Standard_E32-16as",
	"Standard_E32a",
	"Standard_F48s",
	"Standard_ND6s",
	"Standard_NC6s",
	"Standard_EC20ads",
	"Standard_E32-16ads",
	"Standard_E32ads",
	"Standard_E32-8ads",
	"Standard_DC32as",
	"Standard_FX24mds",
	"Standard_HB60-15rs",
	"Standard_HB60-45rs",
	"Standard_HB60-30rs",
	"Standard_NV24s",
	"Standard_HB60rs",
	"Standard_NV12",
	"Standard_E32-8ds",
	"Standard_E32ds",
	"Standard_D48",
	"Standard_E32-16ds",
	"Standard_E32d",
	"Standard_D48s",
	"Standard_D48a",
	"Standard_H16m",
	"Standard_D48ads",
	"Standard_L32s",
	"Standard_DC32ads",
	"Standard_H16mr",
	"Standard_F64s",
	"Standard_M32ts",
	"Standard_D48d",
	"Standard_D48ds",
	"Standard_EC32as",
	"Standard_M32ls",
	"Standard_E48",
	"Standard_E48s",
	"Standard_E48a",
	"Standard_E48as",
	"Standard_F72s",
	"Standard_D64",
	"Standard_D64s",
	"Standard_D64as",
	"Standard_D64a",
	"Standard_M16ms",
	"Standard_M16-8ms",
	"Standard_M16-4ms",
	"Standard_E48ads",
	"Standard_HC44-16rs",
	"Standard_HC44-32rs",
	"Standard_HC44rs",
	"Standard_DC48as",
	"Standard_D64ads",
	"Standard_NP20s",
	"Standard_EC32ads",
	"Standard_FX36mds",
	"Standard_E48d",
	"Standard_E48ds",
	"Standard_HB120-16rs",
	"Standard_HB120-64rs",
	"Standard_HB120rs",
	"Standard_HB120-96rs",
	"Standard_HB120-32rs",
	"Standard_NC24",
	"Standard_E64as",
	"Standard_D64d",
	"Standard_E64",
	"Standard_E64s",
	"Standard_D64ds",
	"Standard_E64-16s",
	"Standard_E64is",
	"Standard_E64-32s",
	"Standard_E64i",
	"Standard_L48s",
	"Standard_DC48ads",
	"Standard_NC24r",
	"Standard_E64-16as",
	"Standard_E64-32as",
	"Standard_E64a",
	"Standard_ND12s",
	"Standard_E64-32ads",
	"Standard_E64ads",
	"Standard_E64-16ads",
	"Standard_EC48as",
	"Standard_DC64as",
	"Standard_NC64as_T4",
	"Standard_FX48mds",
	"Standard_NV24",
	"Standard_NV48s",
	"Standard_D96",
	"Standard_D96s",
	"Standard_E64-32ds",
	"Standard_E64-16ds",
	"Standard_E64d",
	"Standard_E64ds",
	"Standard_D96a",
	"Standard_D96as",
	"Standard_D96ads",
	"Standard_EC48ads",
	"Standard_L64s",
	"Standard_E80is",
	"Standard_DC64ads",
	"Standard_M64ls",
	"Standard_E96as",
	"Standard_D96d",
	"Standard_D96ds",
	"Standard_EC64as",
	"Standard_E80ids",
	"Standard_E96",
	"Standard_E96-48s",
	"Standard_E96-24s",
	"Standard_E96s",
	"Standard_E96-24as",
	"Standard_E96a",
	"Standard_E96-48as",
	"Standard_NC12s",
	"Standard_M32ms",
	"Standard_M32-8ms",
	"Standard_M32-16ms",
	"Standard_M32dms",
	"Standard_L80s",
	"Standard_E96ads",
	"Standard_E96-48ads",
	"Standard_E96-24ads",
	"Standard_M64",
	"Standard_M64s",
	"Standard_DC96as",
	"Standard_NP40s",
	"Standard_EC64ads",
	"Standard_M64ds",
	"Standard_E96ds",
	"Standard_E96-48ds",
	"Standard_E96d",
	"Standard_E96-24ds",
	"Standard_E112ias",
	"Standard_E104i",
	"Standard_E104is",
	"Standard_DC96ads",
	"Standard_E112iads",
	"Standard_E104ids",
	"Standard_E104id",
	"Standard_NC24s",
	"Standard_ND24s",
	"Standard_EC96as",
	"Standard_NC24rs",
	"Standard_ND24rs",
	"Standard_EC96ias",
	"Standard_EC96ads",
	"Standard_M64ms",
	"Standard_M64-32ms",
	"Standard_M64-16ms",
	"Standard_M64m",
	"Standard_M64dms",
	"Standard_EC96iads",
	"Standard_M128",
	"Standard_M128s",
	"Standard_M128ds",
	"Standard_M192is",
	"Standard_M192ids",
	"Standard_ND40rs",
	"Standard_M208s",
	"Standard_M128m",
	"Standard_M128ms",
	"Standard_M128-64ms",
	"Standard_M128-32ms",
	"Standard_M128dms",
	"Standard_ND96asr",
	"Standard_M192ims",
	"Standard_M192idms",
	"Standard_ND96amsr_A100",
	"Standard_M208ms",
	"Standard_M416s",
	"Standard_M416-208s",
	"Standard_M416ms",
	"Standard_M416-208ms",

	// Burstable instances need to be opt in since you
	// don't get 100% of the vCPU capacity all of the time
	// and so we want to avoid surprises unless you ask for it.
	"Standard_B1ls",
	"Standard_B1s",
	"Standard_B1ms",
	"Standard_B2s",
	"Standard_B2ms",
	"Standard_B4ms",
	"Standard_B8ms",
	"Standard_B12ms",
	"Standard_B16ms",
	"Standard_B20ms",

	// Basic instances are less capable than standard
	// ones, so we don't want to be providing them as
	// a default. This is achieved by costing them at
	// a higher price, even though they are cheaper
	// in reality.
	"Basic_A0",
	"Basic_A1",
	"Basic_A2",
	"Basic_A3",
	"Basic_A4",
}

// newInstanceType creates an InstanceType based on a VirtualMachineSize.
func newInstanceType(size compute.VirtualMachineSize) instances.InstanceType {
	// Anything not in the list is more expensive that is in the list.
	cost := len(machineSizeCost)
	sizeName := to.String(size.Name)

	// Actual instance type names often are suffixed with _v3, _v4 etc. We always
	// prefer the highest version number.
	namePart := instSizeVersionRegexp.ReplaceAllString(sizeName, "$name")
	vers := 0
	if namePart == "" {
		namePart = sizeName
	} else {
		versStr := instSizeVersionRegexp.ReplaceAllString(sizeName, "$version")
		if versStr != "" {
			vers, _ = strconv.Atoi(versStr)
		}
	}
	for i, name := range machineSizeCost {
		if namePart == name {
			// Space out the relative costs and make a small subtraction
			// so the higher versions of the same instance have a lower cost.
			cost = 100*i - vers
			break
		}
	}
	if cost == len(machineSizeCost) {
		logger.Debugf("got VM for which we don't have relative cost data: %q", sizeName)
		cost = 100 * cost
	}

	vtype := "Hyper-V"
	return instances.InstanceType{
		Id:       sizeName,
		Name:     sizeName,
		Arches:   []string{arch.AMD64},
		CpuCores: uint64(to.Int32(size.NumberOfCores)),
		Mem:      uint64(to.Int32(size.MemoryInMB)),
		// NOTE(axw) size.OsDiskSizeInMB is the *maximum*
		// OS-disk size. When we create a VM, we can create
		// one that is smaller.
		RootDisk: mbToMib(uint64(to.Int32(size.OsDiskSizeInMB))),
		Cost:     uint64(cost),
		VirtType: &vtype,
		// tags are not currently supported by azure
	}
}

func mbToMib(mb uint64) uint64 {
	b := mb * 1000 * 1000
	return uint64(float64(b) / 1024 / 1024)
}

// findInstanceSpec returns the InstanceSpec that best satisfies the supplied
// InstanceConstraint.
//
// NOTE(axw) for now we ignore simplestreams altogether, and go straight to
// Azure's image registry.
func findInstanceSpec(
	ctx context.ProviderCallContext,
	client compute.VirtualMachineImagesClient,
	instanceTypesMap map[string]instances.InstanceType,
	constraint *instances.InstanceConstraint,
	imageStream string,
) (*instances.InstanceSpec, error) {

	if !constraintHasArch(constraint, arch.AMD64) {
		// Azure only supports AMD64.
		return nil, errors.NotFoundf("%s in arch constraints", arch.AMD64)
	}

	image, err := imageutils.SeriesImage(ctx, constraint.Series, imageStream, constraint.Region, client)
	if err != nil {
		return nil, errors.Trace(err)
	}
	images := []instances.Image{*image}

	instanceTypes := make([]instances.InstanceType, 0, len(instanceTypesMap))
	for _, instanceType := range instanceTypesMap {
		instanceTypes = append(instanceTypes, instanceType)
	}
	constraint.Constraints = defaultToBaselineSpec(constraint.Constraints)
	return instances.FindInstanceSpec(images, constraint, instanceTypes)
}

func constraintHasArch(constraint *instances.InstanceConstraint, arch string) bool {
	for _, constraintArch := range constraint.Arches {
		if constraintArch == arch {
			return true
		}
	}
	return false
}

// If you specify no constraints at all, you're going to get the smallest
// instance type available. In practice that one's a bit small, so unless
// the constraints are deliberately set lower, this gives you a set of
// baseline constraints that are just slightly more ambitious than that.
func defaultToBaselineSpec(constraint constraints.Value) constraints.Value {
	result := constraint
	if !result.HasInstanceType() && result.Mem == nil {
		var value uint64 = defaultMem
		result.Mem = &value
	}
	return result
}
