// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import "github.com/juju/names"

// StorageKind defines the type of the datastore: whether it
// is a raw block device, or a filesystem.
type StorageKind int

const (
	StorageKindUnknown StorageKind = iota
	StorageKindBlock
	StorageKindFilesystem
)

func (k StorageKind) String() string {
	switch k {
	case StorageKindBlock:
		return "block"
	case StorageKindFilesystem:
		return "filesystem"
	default:
		return "unknown"
	}
}

// StorageInstance describes a storage instance, assigned to a service or
// unit.
type StorageInstance struct {
	// Tag is a unique tag assigned by Juju to the storage instance.
	Tag names.StorageTag

	// Kind is the kind of the datastore (block device, filesystem).
	Kind StorageKind
}

// StorageAttachmentInfo provides unit-specific information about a storage
// instance. StorageAttachmentInfo is based on either a volume attachment
// or a filesystem attachment, depending on its kind.
type StorageAttachmentInfo struct {
	// Location is the storage attachment's location: the mount point
	// for a filesystem-kind storage attachment, and the device path
	// for a block-kind.
	Location string
}
