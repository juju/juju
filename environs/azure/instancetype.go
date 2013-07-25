// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"sort"

	"launchpad.net/gwacl"

	"launchpad.net/juju-core/constraints"
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

func (*preferredTypes) isValidArch(arch *string) bool {
	return arch == nil || *arch == "i386" || *arch == "amd64"
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

// selectMachineType returns the Azure machine type that best matches the
// supplied instanceContraint.
func selectMachineType(availableTypes []gwacl.RoleSize, constraint constraints.Value) (*gwacl.RoleSize, error) {
	types := newPreferredTypes(availableTypes)

	if !types.isValidArch(constraint.Arch) {
		return nil, fmt.Errorf("requested unsupported architecture %q", *constraint.Arch)
	}
	if constraint.Container != nil {
		return nil, fmt.Errorf("container type requested, but not supported in Azure: %v", *constraint.Container)
	}

	for _, machineType := range types {
		if types.satisfies(machineType, constraint) {
			return machineType, nil
		}
	}
	return nil, fmt.Errorf("no machine type matches constraints %v", constraint)
}
