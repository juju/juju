// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// DiskParams is a fully specified set of parameters for disk creation,
// derived from one or more of user-specified storage constraints, a
// storage pool definition, and charm storage metadata.
type DiskParams struct {
	// Size is the minimum size of the disk in MiB.
	Size uint64

	// Options is a set of provider-specific options for storage creation,
	// as defined in a storage pool.
	Options map[string]interface{}
}
