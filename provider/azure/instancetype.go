// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/simplestreams"
)

// preferredTypes is a list of machine types, in order of preference so that
// the first type that matches a set of hardware constraints is also the best
// (cheapest) fit for those constraints.  Or if your constraint is a maximum
// price you're willing to pay, your best match is the last type that falls
// within your threshold price.
type preferredTypes []*gwacl.RoleSize

// preferredTypes implements sort.Interface.
var _ sort.Interface = (*preferredTypes)(nil)

// newPreferredTypes creates a preferredTypes based on the given slice of
// RoleSize objects.  It will hold pointers to the elements of that slice.
func newPreferredTypes(availableTypes []gwacl.RoleSize) preferredTypes {
	types := make(preferredTypes, len(availableTypes))
	for index := range availableTypes {
		types[index] = &availableTypes[index]
	}
	sort.Sort(&types)
	return types
}

// Len is specified in sort.Interface.
func (types *preferredTypes) Len() int {
	return len(*types)
}

// Less is specified in sort.Interface.
func (types *preferredTypes) Less(i, j int) bool {
	// All we care about for now is cost.  If at some point Azure offers
	// different tradeoffs for the same price, we may need a tie-breaker.
	return (*types)[i].Cost < (*types)[j].Cost
}

// Swap is specified in sort.Interface.
func (types *preferredTypes) Swap(i, j int) {
	firstPtr := &(*types)[i]
	secondPtr := &(*types)[j]
	*secondPtr, *firstPtr = *firstPtr, *secondPtr
}

// suffices returns whether the given value is high enough to meet the
// required value, if any.  If "required" is nil, there is no requirement
// and so any given value will do.
//
// This is a method only to limit namespace pollution.  It ignores the receiver.
func (*preferredTypes) suffices(given uint64, required *uint64) bool {
	return required == nil || given >= *required
}

// satisfies returns whether the given machine type is enough to satisfy
// the given constraints.  (It doesn't matter if it's overkill; all that
// matters here is whether the machine type is good enough.)
//
// This is a method only to limit namespace pollution.  It ignores the receiver.
func (types *preferredTypes) satisfies(machineType *gwacl.RoleSize, constraint constraints.Value) bool {
	// gwacl does not model CPU power yet, although Azure does have the
	// option of a shared core (for ExtraSmall instances).  For now we
	// just pretend that's a full-fledged core.
	return types.suffices(machineType.CpuCores, constraint.CpuCores) &&
		types.suffices(machineType.Mem, constraint.Mem)
}

const defaultMem = 1 * gwacl.GB

// If you specify no constraints at all, you're going to get the smallest
// instance type available.  In practice that one's a bit small.  So unless
// the constraints are deliberately set lower, this gives you a set of
// baseline constraints that are just slightly more ambitious than that.
func defaultToBaselineSpec(constraint constraints.Value) constraints.Value {
	result := constraint
	if result.Mem == nil {
		var value uint64 = defaultMem
		result.Mem = &value
	}
	return result
}

// selectMachineType returns the Azure machine type that best matches the
// supplied instanceConstraint.
func selectMachineType(availableTypes []gwacl.RoleSize, constraint constraints.Value) (*gwacl.RoleSize, error) {
	types := newPreferredTypes(availableTypes)
	for _, machineType := range types {
		if types.satisfies(machineType, constraint) {
			return machineType, nil
		}
	}
	return nil, fmt.Errorf("no machine type matches constraints %v", constraint)
}

// getEndpoint returns the simplestreams endpoint to use for the given Azure
// location (e.g. West Europe or China North).
func getEndpoint(location string) string {
	// Simplestreams uses the management-API endpoint for the image, not
	// the base managent API URL.
	return gwacl.GetEndpoint(location).ManagementAPI()
}

// As long as this code only supports the default simplestreams
// database, which is always signed, there is no point in accepting
// unsigned metadata.
//
// For tests, however, unsigned data is more convenient.  They can override
// this setting.
var signedImageDataOnly = true

// findMatchingImages queries simplestreams for OS images that match the given
// requirements.
//
// If it finds no matching images, that's an error.
func findMatchingImages(e *azureEnviron, location, series string, arches []string) ([]*imagemetadata.ImageMetadata, error) {
	endpoint := getEndpoint(location)
	constraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{location, endpoint},
		Series:    []string{series},
		Arches:    arches,
		Stream:    e.Config().ImageStream(),
	})
	sources, err := imagemetadata.GetMetadataSources(e)
	if err != nil {
		return nil, err
	}
	indexPath := simplestreams.DefaultIndexPath
	images, _, err := imagemetadata.Fetch(sources, indexPath, constraint, signedImageDataOnly)
	if len(images) == 0 || errors.IsNotFound(err) {
		return nil, fmt.Errorf("no OS images found for location %q, series %q, architectures %q (and endpoint: %q)", location, series, arches, endpoint)
	} else if err != nil {
		return nil, err
	}
	return images, nil
}

// newInstanceType creates an InstanceType based on a gwacl.RoleSize.
func newInstanceType(roleSize gwacl.RoleSize) instances.InstanceType {
	vtype := "Hyper-V"
	// Actually Azure has shared and dedicated CPUs, but gwacl doesn't
	// model that distinction yet.
	var cpuPower uint64 = 100

	return instances.InstanceType{
		Id:       roleSize.Name,
		Name:     roleSize.Name,
		CpuCores: roleSize.CpuCores,
		Mem:      roleSize.Mem,
		RootDisk: roleSize.OSDiskSpaceVirt,
		Cost:     roleSize.Cost,
		VirtType: &vtype,
		CpuPower: &cpuPower,
		// tags are not currently supported by azure
	}
}

// listInstanceTypes describes the available instance types based on a
// description in gwacl's terms.
func listInstanceTypes(env *azureEnviron, roleSizes []gwacl.RoleSize) ([]instances.InstanceType, error) {
	arches, err := env.SupportedArchitectures()
	if err != nil {
		return nil, err
	}
	types := make([]instances.InstanceType, len(roleSizes))
	for index, roleSize := range roleSizes {
		types[index] = newInstanceType(roleSize)
		types[index].Arches = arches
	}
	return types, nil
}

// findInstanceSpec returns the InstanceSpec that best satisfies the supplied
// InstanceConstraint.
func findInstanceSpec(env *azureEnviron, constraint *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	constraint.Constraints = defaultToBaselineSpec(constraint.Constraints)
	imageData, err := findMatchingImages(env, constraint.Region, constraint.Series, constraint.Arches)
	if err != nil {
		return nil, err
	}
	images := instances.ImageMetadataToImages(imageData)
	instanceTypes, err := listInstanceTypes(env, gwacl.RoleSizes)
	if err != nil {
		return nil, err
	}
	return instances.FindInstanceSpec(images, constraint, instanceTypes)
}
