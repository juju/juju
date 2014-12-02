// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// DatastoreId is a Juju-internal identifier for a datastore.
type DatastoreId string

// DatastoreKind defines the type of the datastore: whether it
// is a raw block device, or a filesystem.
type DatastoreKind int

const (
	DatastoreKindUnknown DatastoreKind = iota
	DatastoreKindBlock
	DatastoreKindFilesystem
)

func (k DatastoreKind) String() string {
	switch k {
	case DatastoreKindBlock:
		return "block"
	case DatastoreKindFilesystem:
		return "filesystem"
	default:
		return "unknown"
	}
}

// Datastore describes a datastore assigned to a service unit.
type Datastore struct {
	// Id is an identifier assigned by Juju to the datastore.
	Id DatastoreId `yaml:"id"`

	// Name is the storage name associated with the datastore.
	Name string `yaml:"name"`

	// Kind is the kind of the datastore (block device, filesystem).
	Kind DatastoreKind `yaml:"kind"`

	// Location is the location of the datastore. The exact meaning
	// of this depends on the datastore type.
	//
	// For block devices, the location is the path to the block device;
	// for filesystems, the location is the mount point.
	Location string `yaml:"location"`

	// Specification describes parameters for creating the datastore if
	// it is not yet attached. Exactly how the datastore is created is
	// source-dependent.
	Specification *Specification `yaml:"specification,omitempty"`

	// Filesystem describes the filesystem properties of the datastore,
	// for filesystem-type datastores. This will be non-nil only after
	// the filesystem has been created.
	Filesystem *Filesystem `yaml:"filesystem,omitempty"`
}

// BlockDeviceId is a source-specific identifier for a block device
// (e.g. EBS volume ID).
//
// BlockDeviceId values are required to be globally unique, so if the
// IDs allocated by a source are only unique to the source, the ID must
// incorporate a unique identifier for the source itself.
type BlockDeviceId string

// BlockDevice describes a block device (disk, logical volume, etc.)
type BlockDevice struct {
	// Id is an identifier assigned by the block device source.
	Id BlockDeviceId `yaml:"id"`

	// DeviceName is the block device's OS-specific name (e.g. "sdb").
	DeviceName string `yaml:"devicename,omitempty"`

	// Label is the label for the filesystem on the block device.
	//
	// This will be empty if the block device does not have a filesystem,
	// or if the filesystem is not yet known to Juju.
	Label string `yaml:"label,omitempty"`

	// UUID is a unique identifier for the filesystem on the block device.
	//
	// This will be empty if the block device does not have a filesystem,
	// or if the filesystem is not yet known to Juju.
	//
	// The UUID format is not necessarily uniform; for example, LVM UUIDs
	// differ in format to the standard v4 UUIDs.
	UUID string `yaml:"uuid,omitempty"`

	// Size is the size of the block device, in MiB.
	Size uint64 `yaml:"size"`

	// InUse indicates that the block device is in use (e.g. mounted).
	InUse bool `yaml:"inuse"`
}

// Filesystem defines the type and mount options that should be used
// to mount a filesystem.
type Filesystem struct {
	Type         string   `yaml:"type"`
	MountOptions []string `yaml:"mountoptions,omitempty"`
}
