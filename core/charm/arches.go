// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"sort"
)

// Arch represents a platform architecture.
type Arch string

func (a Arch) String() string {
	return string(a)
}

const (
	// ArchAll represents an architecture for all architectures.
	ArchAll Arch = "all"

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
	set map[Arch]struct{}
}

// DefaultArches creates a series of arches to compare against.
func DefaultArches() Arches {
	return Arches{
		set: map[Arch]struct{}{
			ArchAll:   {},
			ArchAMD64: {},
			ArchARM64: {},
			ArchPPC64: {},
			ArchS390:  {},
		},
	}
}

// Contains checks to see if a given arch is found with in the list.
func (a Arches) Contains(arch Arch) bool {
	_, ok := a.set[arch]
	return ok
}

// StringList returns an ordered list of strings.
// ArchAll will always be the front of the slice to show importance of the enum
// value.
func (a Arches) StringList() []string {
	var prependAll bool

	list := make([]string, 0)
	for arch := range a.set {
		if arch == ArchAll {
			prependAll = true
			continue
		}
		list = append(list, string(arch))
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i] < list[j]
	})

	if !prependAll {
		return list
	}

	return append([]string{string(ArchAll)}, list...)
}
