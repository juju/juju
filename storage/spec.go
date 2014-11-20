// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// DefaultFilesystemType is the default filesystem type to create on a
// block device if no preferences are specified or viable.
const DefaultFilesystemType = "ext4"

// Specification is a fully specified set of requirements for storage,
// derived from a Directive and a charm's storage metadata.
type Specification struct {
	// Name is the name of the storage.
	Name string

	// Source is the storage source (provider, ceph, ...).
	Source string

	// Size is the size of the storage in MiB.
	Size uint64

	// Options is source-specific options for storage creation.
	Options string

	// ReadOnly indicates that the storage should be made read-only if
	// possible.
	ReadOnly bool

	// Persistent indicates that the storage should be made persistent,
	// beyond the lifetime of the entity it is attached to, if possible.
	Persistent bool

	// FilesystemPreferences defines the preferences of filesystems to
	// create/use. If none are specified, Juju will use DefaultFilesystemType.
	FilesystemPreferences []FilesystemPreference
}

// FilesystemPreference describes a filesystem to attempt to create
// on a block device, and any options that should be used to do so
// and later mount it.
type FilesystemPreference struct {
	Filesystem
	MkfsOptions []string `yaml:"mkfsoptions,omitempty"`
}
