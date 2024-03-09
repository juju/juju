// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	EnsureStoragePoolFilter = (*StorageAPI).ensureStoragePoolFilter
	NewStorageAPIForTest    = NewStorageAPI
)

type (
	StorageVolume = storageVolume
	StorageFile   = storageFile
)
