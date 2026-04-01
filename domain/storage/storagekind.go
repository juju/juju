// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// StorageKind represents a unique value to signal what kind a storage instance
// in the model is. It directly communicates the composition of the storage
// instance to aid in looking below the waterline. Initially when storage is
// created in the model it is created to satisfy the type of storage requested
// by a charm.
//
// While a charm storage type is a signal into the determination of a storage
// kind it is not a direct mapping and MUST never be relied on to communicate a
// charms intent. The mapping between charm storage type and storage kind is
// maintained in business logic that owns this mapping.
type StorageKind int

const (
	// KindBlock represents storage in the model that is a raw block device for
	// use.
	StorageKindBlock StorageKind = iota

	// KindFilesystem represents storage in the model that is a filesystem.
	StorageKindFilesystem
)

// String returns the string representation of this [StorageKind]. The value
// return should not be considered a const enum value that can be relied upon
// for comparison.
//
// Value is intended purely for display purposes. If the kind is unknown an
// empty string is returned.
//
// Implements the [fmt.Stringer] interface.
func (s StorageKind) String() string {
	switch s {
	case StorageKindBlock:
		return "block"
	case StorageKindFilesystem:
		return "filesystem"
	default:
		return "unknown"
	}
}
