// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"strings"

	"github.com/juju/collections/set"
)

// Arch represents a platform architecture.
type Arch = string

const (
	// ArchAMD64 defines a amd64 architecture.
	ArchAMD64 Arch = "amd64"

	// ArchARM64 defines a arm64 architecture.
	ArchARM64 Arch = "arm64"

	// ArchPPC64 defines a ppc64 architecture.
	ArchPPC64 Arch = "ppc64"

	// ArchS390 defines a s390 architecture.
	ArchS390 Arch = "s390"
)

// Arches defines a list of arches to compare against.
type Arches struct {
	set set.Strings
}

// AllArches creates a series of arches to compare against.
func AllArches() Arches {
	return Arches{
		set: set.NewStrings(ArchAMD64, ArchARM64, ArchPPC64, ArchS390),
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
