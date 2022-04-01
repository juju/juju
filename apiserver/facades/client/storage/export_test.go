// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	ValidatePoolListFilter   = (*StorageAPI).validatePoolListFilter
	ValidateNameCriteria     = (*StorageAPI).validateNameCriteria
	ValidateProviderCriteria = (*StorageAPI).validateProviderCriteria
	EnsureStoragePoolFilter  = (*StorageAPI).ensureStoragePoolFilter
	NewStorageAPIForTest     = NewStorageAPI
)

type (
	StorageVolume = storageVolume
	StorageFile   = storageFile
)
