// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// StorageKind represents the kind of storage
// as recorded in the storage kind lookup table.
type StorageKind int

const (
	StorageKindBlock StorageKind = iota
	StorageKindFilesystem
)
