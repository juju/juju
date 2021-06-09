// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package arch

import (
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/utils/arch"
)

const (
	// DefaultArchitecture represents the default architecture we expect to use
	// if none is present.
	DefaultArchitecture = arch.AMD64
)

// Arch represents a platform architecture.
type Arch = string

// Arches defines a list of arches to compare against.
type Arches struct {
	set set.Strings
}

// AllArches creates a series of arches to compare against.
func AllArches() Arches {
	return Arches{
		set: set.NewStrings(arch.AllSupportedArches...),
	}
}

// Contains checks to see if a given arch is found with in the list.
func (a Arches) Contains(arch Arch) bool {
	return a.set.Contains(arch)
}

// StringList returns an ordered list of strings.
// ArchAll will always be the front of the slice to show importance of the enum
// value.
func (a Arches) StringList() []string {
	return a.set.SortedValues()
}

func (a Arches) String() string {
	return strings.Join(a.set.SortedValues(), ",")
}

// ConstraintArch returns the arch for the constraint if there is one,
// else it returns the default arch.
func ConstraintArch(cons constraints.Value, defaultCons *constraints.Value) string {
	if cons.HasArch() {
		return *cons.Arch
	}
	if defaultCons != nil && defaultCons.HasArch() {
		return *defaultCons.Arch
	}
	return DefaultArchitecture
}
