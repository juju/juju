// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	"github.com/juju/errors"

	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/azure/internal/imageutils"
)

const defaultMem = 1024 // 1GiB

var instSizeVersionRegexp = regexp.MustCompile(`^((?P<instType>(Standard|Basic))_)?(?P<name>[^_]*)(?:_v(?P<version>\d\d?))?(_Promo)?$`)

// The ordering here is for linux vm costs for the eastus region
// obtained from https://azureprice.net. This is used as a fallback
// where real costs are not available, to ensure we choose the right
// VMs given matching constraints. The assumption is that all regions
// have the same relative costs.
//
// xxxS is the same price as xxx, but is targeted at Premium Storage.
// We put the premium storage variants directly after their non-premium counterparts.
var machineSizeCost = []string{
	"A0",
	"A1",
	"F1",
	"F1s",
	"DS1",
	"D1",
	"D2as",
	"A2",
	"D2",
	"D2s",
	"D2a",
	"DC1s",
	"DC1ds",
	"F2",
	"F2s",
	"D2ads",
	"E2as",
	"D2d",
	"D2ds",
	"A2m",
	"E2",
	"E2s",
	"E2a",
	"E2ads",
	"DC2as",
	"E2d",
	"E2ds",
	"E2bs",
	"E2bds",
	"DS2",
	"DC2ads",
	"EC2as",
	"D11",
	"DS11",
	"DS11-1",
	"A4",
	"D4",
	"D4a",
	"D4s",
	"DC2s",
	"DC2ds",
	"D4as",
	"F4",
	"F4s",
	"D4ads",
	"EC2ads",
	"D4d",
	"D4ds",
	"NV4as",
	"A4m",
	"A3",
	"A5",
	"E4",
	"E4s",
	"E4as",
	"E4-2s",
	"E4a",
	"E4ads",
	"E4-2as",
	"E4-2ads",
	"DC4as",
	"E4-2ds",
	"E4d",
	"E4ds",
	"E4bs",
	"E4bds",
	"D3",
	"DS3",
	"DC4ads",
	"EC4as",
	"D12",
	"DS12",
	"DS12-1",
	"DS12-2",
	"FX4mds",
	"D8",
	"D8s",
	"D8a",
	"DC4s",
	"DC4ds",
	"D8as",
	"F8",
	"F8s",
	"A8",
	"D8ads",
	"EC4ads",
	"D8d",
	"D8ds",
	"NV8as",
	"A8m",
	"A6",
	"E8",
	"E8s",
	"E8-2s",
	"E8-4s",
	"E8a",
	"E8-4as",
	"E8as",
	"E8-2as",
	"E8ads",
	"E8-2ads",
	"E8-4ads",
	"NC4as_T4",
	"DC8as",
	"E8d",
	"E8ds",
	"E8bs",
	"E8bds",
	"E8-4ds",
	"E8-2ds",
	"DS4",
	"L8s",
	"DC8ads",
	"EC8as",
	"DS13-4",
	"D13",
	"DS13-2",
	"NC8as_T4",
	"D16",
	"D16s",
	"D16as",
	"DC8",
	"DC8s",
	"DC8ds",
	"D16a",
	"DS13",
	"F16",
	"F16s",
	"D16ads",
	"PB6s",
	"EC8ads",
	"NC6",
	"D16d",
	"D16ds",
	"H8",
	"NV16as",
	"A7",
	"E16",
	"E16s",
	"E16-4s",
	"E16-8s",
	"E16a",
	"E16-8as",
	"E16as",
	"E16-4as",
	"E16-4ads",
	"E16ads",
	"E16-8ads",
	"DC16s",
	"DC16ds",
	"DC16as",
	"FX12mds",
	"NV6",
	"NV12s",
	"E16d",
	"E16ds",
	"E16bs",
	"E16bds",
	"E16-4ds",
	"E16-8ds",
	"DS5",
	"D5",
	"NC16as_T4",
	"H8m",
	"L16s",
	"E20",
	"E20s",
	"E20a",
	"E20as",
	"DC16ads",
	"E20ads",
	"F32s",
	"EC16as",
	"E20d",
	"E20ds",
	"D14",
	"DS14",
	"DS14-8",
	"DS14-4",
	"D32",
	"D32s",
	"D32as",
	"D32a",
	"M8-2ms",
	"M8-4ms",
	"M8ms",
	"D32ads",
	"NP10s",
	"EC16ads",
	"EC20as",
	"NC12",
	"H16",
	"D32d",
	"D32ds",
	"D15",
	"DS15",
	"NV32as",
	"H16r",
	"E32",
	"E32-8s",
	"E32-16s",
	"E32s",
	"E32-8as",
	"E32as",
	"E32-16as",
	"E32a",
	"F48s",
	"ND6s",
	"NC6s",
	"EC20ads",
	"E32-16ads",
	"E32ads",
	"E32-8ads",
	"DC24s",
	"DC24ds",
	"DC32s",
	"DC32ds",
	"DC32as",
	"FX24mds",
	"HB60-15rs",
	"HB60-45rs",
	"HB60-30rs",
	"NV24s",
	"HB60rs",
	"NV12",
	"E32-8ds",
	"E32ds",
	"D48",
	"E32-16ds",
	"E32d",
	"E32bs",
	"E32bds",
	"D48s",
	"D48a",
	"H16m",
	"D48as",
	"D48ads",
	"L32s",
	"DC32ads",
	"H16mr",
	"F64s",
	"M32ts",
	"D48d",
	"D48ds",
	"EC32as",
	"M32ls",
	"E48",
	"E48s",
	"E48a",
	"E48as",
	"F72s",
	"D64",
	"D64s",
	"D64as",
	"D64a",
	"M16ms",
	"M16-8ms",
	"M16-4ms",
	"E48ads",
	"HC44-16rs",
	"HC44-32rs",
	"HC44rs",
	"DC48as",
	"DC48s",
	"DC48ds",
	"D64ads",
	"NP20s",
	"EC32ads",
	"FX36mds",
	"E48d",
	"E48ds",
	"E48bs",
	"E48bds",
	"HB120-16rs",
	"HB120-64rs",
	"HB120rs",
	"HB120-96rs",
	"HB120-32rs",
	"NC24",
	"E64as",
	"D64d",
	"E64",
	"E64s",
	"D64ds",
	"E64-16s",
	"E64is",
	"E64-32s",
	"E64i",
	"L48s",
	"DC48ads",
	"NC24r",
	"E64-16as",
	"E64-32as",
	"E64a",
	"ND12s",
	"E64-32ads",
	"E64ads",
	"E64-16ads",
	"EC48as",
	"DC64as",
	"NC64as_T4",
	"FX48mds",
	"NV24",
	"NV48s",
	"D96",
	"D96s",
	"E64-32ds",
	"E64-16ds",
	"E64d",
	"E64ds",
	"E64bs",
	"E64bds",
	"D96a",
	"D96as",
	"D96ads",
	"EC48ads",
	"L64s",
	"E80is",
	"DC64ads",
	"M64ls",
	"E96as",
	"E96ias",
	"D96d",
	"D96ds",
	"EC64as",
	"E80ids",
	"E96",
	"E96-48s",
	"E96-24s",
	"E96s",
	"E96-24as",
	"E96a",
	"E96-48as",
	"NC12s",
	"M32ms",
	"M32-8ms",
	"M32-16ms",
	"M32dms",
	"L80s",
	"E96ads",
	"E96-48ads",
	"E96-24ads",
	"M64",
	"M64s",
	"DC96as",
	"NP40s",
	"EC64ads",
	"M64ds",
	"E96ds",
	"E96-48ds",
	"E96d",
	"E96-24ds",
	"E112ias",
	"E104i",
	"E104is",
	"DC96ads",
	"E112iads",
	"E104ids",
	"E104id",
	"NC24s",
	"ND24s",
	"EC96as",
	"NC24rs",
	"ND24rs",
	"EC96ias",
	"EC96ads",
	"M64ms",
	"M64-32ms",
	"M64-16ms",
	"M64m",
	"M64dms",
	"EC96iads",
	"M128",
	"M128s",
	"M128ds",
	"M192is",
	"M192ids",
	"ND40rs",
	"M208s",
	"M128m",
	"M128ms",
	"M128-64ms",
	"M128-32ms",
	"M128dms",
	"ND96asr",
	"M192ims",
	"M192idms",
	"ND96amsr_A100",
	"M208ms",
	"M416s",
	"M416-208s",
	"M416ms",
	"M416-208ms",

	// Burstable instances need to be opt in since you
	// don't get 100% of the vCPU capacity all of the time
	// and so we want to avoid surprises unless you ask for it.
	"B1ls",
	"B1s",
	"B1ms",
	"B2s",
	"B2ms",
	"B4ms",
	"B8ms",
	"B12ms",
	"B16ms",
	"B20ms",
}

// newInstanceType creates an InstanceType based on a VirtualMachineSize.
func newInstanceType(arch corearch.Arch, size armcompute.VirtualMachineSize) instances.InstanceType {
	sizeName := toValue(size.Name)
	// Actual instance type names often are suffixed with _v3, _v4 etc. We always
	// prefer the highest version number.
	namePart := instSizeVersionRegexp.ReplaceAllString(sizeName, "$name")
	instType := instSizeVersionRegexp.ReplaceAllString(sizeName, "$instType")
	isPromo := strings.HasSuffix(sizeName, "_Promo")
	vers := 0
	if namePart == "" || instType == "Basic" {
		namePart = sizeName
	} else {
		versStr := instSizeVersionRegexp.ReplaceAllString(sizeName, "$version")
		if versStr != "" {
			vers, _ = strconv.Atoi(versStr)
		}
	}

	var (
		cost  int
		found bool
	)
	// We don't have proper cost info for promo instances, so don't try and rank them.
	if !isPromo {
		for i, name := range machineSizeCost {
			if namePart == name {
				// Space out the relative costs and make a small subtraction
				// so the higher versions of the same instance have a lower cost.
				cost = 100*i - vers
				found = true
				break
			}
		}
	}
	// Anything not in the list is more expensive that is in the list.
	if !found {
		if !isPromo && instType != "Basic" {
			logger.Debugf(context.TODO(), "got VM for which we don't have relative cost data: %q", sizeName)
		}
		cost = 100 * len(machineSizeCost)
	}

	vtype := "Hyper-V"
	return instances.InstanceType{
		Id:       sizeName,
		Name:     sizeName,
		Arch:     arch,
		CpuCores: uint64(toValue(size.NumberOfCores)),
		Mem:      uint64(toValue(size.MemoryInMB)),
		// NOTE(axw) size.OsDiskSizeInMB is the *maximum*
		// OS-disk size. When we create a VM, we can create
		// one that is smaller.
		RootDisk: mbToMib(uint64(toValue(size.OSDiskSizeInMB))),
		Cost:     uint64(cost),
		VirtType: &vtype,
		// tags are not currently supported by azure
	}
}

func deleteInstanceFamily(instanceTypes map[string]instances.InstanceType, fullName string) {
	toDeleteNamePart := instSizeVersionRegexp.ReplaceAllString(fullName, "$name")
	for n := range instanceTypes {
		namePart := instSizeVersionRegexp.ReplaceAllString(n, "$name")
		if namePart != "" && namePart == toDeleteNamePart || n == toDeleteNamePart {
			delete(instanceTypes, n)
		}
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
func (env *azureEnviron) findInstanceSpec(
	ctx context.Context,
	instanceTypesMap map[string]instances.InstanceType,
	constraint *instances.InstanceConstraint,
	imageStream string,
	preferGen1Image bool,
) (*instances.InstanceSpec, error) {
	if !constraint.Constraints.HasArch() && constraint.Arch != "" {
		constraint.Constraints.Arch = &constraint.Arch
	}

	client, err := env.imagesClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	image, err := imageutils.BaseImage(ctx, env.CredentialInvalidator, constraint.Base, imageStream, constraint.Region, constraint.Arch, client, preferGen1Image)
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
