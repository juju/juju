// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/juju/errors"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/azure/internal/imageutils"
)

const defaultMem = 1024 // 1GiB

// newInstanceType creates an InstanceType based on a VirtualMachineSize.
func newInstanceType(size compute.VirtualMachineSize) instances.InstanceType {
	// We're not doing real costs for now; just made-up, relative
	// costs, to ensure we choose the right VMs given matching
	// constraints. This was based on the pricing for West US,
	// and assumes that all regions have the same relative costs.
	//
	// DS is the same price as D, but is targeted at Premium Storage.
	// Likewise for GS and G. We put the premium storage variants
	// directly after their non-premium counterparts.
	machineSizeCost := []string{
		"Standard_A0",
		"Standard_A1",
		"Standard_D1",
		"Standard_DS1",
		"Standard_D1_v2",
		"Standard_A2",
		"Standard_D2",
		"Standard_DS2",
		"Standard_D2_v2",
		"Standard_D11",
		"Standard_DS11",
		"Standard_D11_v2",
		"Standard_A3",
		"Standard_D3",
		"Standard_DS3",
		"Standard_D3_v2",
		"Standard_D12",
		"Standard_DS12",
		"Standard_D12_v2",
		"Standard_A5", // Yes, A5 is cheaper than A4.
		"Standard_A4",
		"Standard_A6",
		"Standard_G1",
		"Standard_GS1",
		"Standard_D4",
		"Standard_DS4",
		"Standard_D4_v2",
		"Standard_D13",
		"Standard_DS13",
		"Standard_D13_v2",
		"Standard_A7",
		"Standard_A10",
		"Standard_G2",
		"Standard_GS2",
		"Standard_D5_v2",
		"Standard_D14",
		"Standard_DS14",
		"Standard_D14_v2",
		"Standard_A8",
		"Standard_A11",
		"Standard_G3",
		"Standard_GS3",
		"Standard_A9",
		"Standard_G4",
		"Standard_GS4",
		"Standard_GS5",
		"Standard_G5",

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

	// Anything not in the list is more expensive that is in the list.
	cost := len(machineSizeCost)
	sizeName := to.String(size.Name)
	for i, name := range machineSizeCost {
		if sizeName == name {
			cost = i
			break
		}
	}
	if cost == len(machineSizeCost) {
		logger.Warningf("found unknown VM size %q", sizeName)
	}

	vtype := "Hyper-V"
	return instances.InstanceType{
		Id:       sizeName,
		Name:     sizeName,
		Arches:   []string{arch.AMD64},
		CpuCores: uint64(to.Int(size.NumberOfCores)),
		Mem:      uint64(to.Int(size.MemoryInMB)),
		// NOTE(axw) size.OsDiskSizeInMB is the maximum root disk
		// size, but the actual disk size is limited to the size
		// of the image/VHD that the machine is backed by. The
		// Azure Resource Manager APIs do not provide a way of
		// determining the image size.
		//
		// All of the published images that we use are ~30GiB.
		RootDisk: uint64(29495),
		Cost:     uint64(cost),
		VirtType: &vtype,
		// tags are not currently supported by azure
	}
}

// findInstanceSpec returns the InstanceSpec that best satisfies the supplied
// InstanceConstraint.
//
// NOTE(axw) for now we ignore simplestreams altogether, and go straight to
// Azure's image registry.
func findInstanceSpec(
	client compute.VirtualMachineImagesClient,
	instanceTypesMap map[string]instances.InstanceType,
	constraint *instances.InstanceConstraint,
	imageStream string,
) (*instances.InstanceSpec, error) {

	if !constraintHasArch(constraint, arch.AMD64) {
		// Azure only supports AMD64.
		return nil, errors.NotFoundf("%s in arch constraints", arch.AMD64)
	}

	image, err := imageutils.SeriesImage(constraint.Series, imageStream, constraint.Region, client)
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
