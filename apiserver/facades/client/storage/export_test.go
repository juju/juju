// Copyright 2015, 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	ValidatePoolListFilter   = (*APIv5).validatePoolListFilter
	ValidateNameCriteria     = (*APIv5).validateNameCriteria
	ValidateProviderCriteria = (*APIv5).validateProviderCriteria
	EnsureStoragePoolFilter  = (*APIv5).ensureStoragePoolFilter
)

type (
	StorageVolume = storageVolume
	StorageFile   = storageFile
)
