// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	domainstorage "github.com/juju/juju/domain/storage"
)

// DefaultStorageProvisioners defines the set of default storage provisioners
// for each type of storage that can be provisioned in a model. If a storage
// type has no default provisioner set then a default does not exist for the
// model.
type DefaultStorageProvisioners struct {
	// BlockdevicePoolUUID describes the storage pool uuid that should be used
	// when provisioning new block device storage in the model. If this value is
	// set then [defaultStorageProvisioners.BlockdeviceProviderType] will not be
	// set.
	BlockdevicePoolUUID *domainstorage.StoragePoolUUID

	// BlockdeviceProviderType describes the storage provider type that should
	// be used when provisioning new block device storage in the model. If this
	// value is set then [defaultStorageProvisioners.BlockdevicePoolUUID] will
	// not be set.
	BlockdeviceProviderType *string

	// FilesystemPoolUUID describes the storage pool uuid that should be used
	// when provisioning new filesystem storage in the model. If this value is
	// set then [defaultStorageProvisioners.FilesystemProviderType] will not be
	// set.
	FilesystemPoolUUID *domainstorage.StoragePoolUUID

	// FilesystemProviderType describes the storage provider type that should
	// be used when provisioning new filesystem storage in the model. If this
	// value is set then [defaultStorageProvisioners.FilesystemPoolUUID] will
	// not be set.
	FilesystemProviderType *string
}
