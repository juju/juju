// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

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
	// Name is a unique name assigned by Juju to the datastore.
	Name string `yaml:"name"`

	// Kind is the kind of the datastore (block device, filesystem).
	Kind DatastoreKind `yaml:"kind"`

	// Specification describes parameters for creating the datastore if
	// it is not yet attached. Exactly how the datastore is created is
	// source-dependent.
	Specification *Specification `yaml:"specification,omitempty"`

	// Filesystem describes the filesystem properties of the datastore,
	// for filesystem-type datastores. This will be non-nil only after
	// the filesystem has been created.
	Filesystem *Filesystem `yaml:"filesystem,omitempty"`
}

// Filesystem defines the type and mount options that should be used
// to mount a filesystem.
type Filesystem struct {
	Type         string   `yaml:"type"`
	MountOptions []string `yaml:"mountoptions,omitempty"`
}
