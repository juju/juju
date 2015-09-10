// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	GetStorageShowAPI    = &getStorageShowAPI
	GetStorageListAPI    = &getStorageListAPI
	GetPoolListAPI       = &getPoolListAPI
	GetPoolCreateAPI     = &getPoolCreateAPI
	GetVolumeListAPI     = &getVolumeListAPI
	GetFilesystemListAPI = &getFilesystemListAPI

	ConvertToVolumeInfo     = convertToVolumeInfo
	ConvertToFilesystemInfo = convertToFilesystemInfo
	GetStorageAddAPI        = &getStorageAddAPI
)
