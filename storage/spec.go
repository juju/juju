// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

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
}
