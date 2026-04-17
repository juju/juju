// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import "github.com/juju/juju/domain/application/charm"

// CharmStorageDefinition holds the information required about a
// Charm's Storage Definition for the purpose of validating storage actions
// against the definition.
//
// This type exist because there are many places where storage related
// operations need to be validated against the Charm's Storage Definition for
// correctness. The types representing the Charm's Storage Definition vary so
// this type acts a common representation for the purposes of validation.
type CharmStorageDefinition struct {
	// CountMax is the maximum number of Storage Instances that can be attached
	// of this definition per Unit. If the Charm has no opinion this value will
	// be -1.
	CountMax int

	// CountMin is the minimum number of storage instances that MUST be attached
	// for this storage definition per unit. If the charm has no opinion this
	// value will be 0.
	CountMin int

	// MinimumSize is the minimum size a Storage Instance must be to be able to
	// be attached to a unit fulfilling this Storage Definition. The value is
	// expressed MiB.
	MinimumSize uint64

	// Name is the name of the storage definition defined by the Charm.
	Name string

	// Shared indicates the storage definition is shared between units.
	Shared bool

	// Type is the storage type define by the Charm's storage definition.
	// Expected values will be one of [charm.StorageBlock] or
	// [charm.StorageFilesystem].
	Type charm.StorageType
}
