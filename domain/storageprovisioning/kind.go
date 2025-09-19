// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

// Kind represents a unique value to signal what kind a storage is within the
// model. This value is tightly coupled to the storage kind values used for
// charms.
type Kind int

const (
	// KindBlock represents storage in the model that is a raw block device for
	// use.
	KindBlock Kind = iota

	// KindFilesystem represents storage in the model that is a filesystem.
	KindFilesystem
)
