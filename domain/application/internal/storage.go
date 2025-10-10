// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	domainstorage "github.com/juju/juju/domain/storage"
)

// ModelStoragePools provides the default storage pools that have been set
// within the model. If a value is nil then no default exists.
type ModelStoragePools struct {
	// BlockDevicePoolUUID provides the storage pool uuid to use for new block
	// storage.
	BlockDevicePoolUUID *domainstorage.StoragePoolUUID

	// FilesystemPoolUUID provides the storage pool uuid to use for
	// filesystem storage.
	FilesystemPoolUUID *domainstorage.StoragePoolUUID
}
