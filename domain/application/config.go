// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

// TODO(dqlite) - we don't want to reference environs/config here but need a place for config key names.
const (
	// StorageDefaultBlockSourceKey is the key for the default block storage source.
	StorageDefaultBlockSourceKey = "storage-default-block-source"

	// StorageDefaultFilesystemSourceKey is the key for the default filesystem storage source.
	StorageDefaultFilesystemSourceKey = "storage-default-filesystem-source"
)
