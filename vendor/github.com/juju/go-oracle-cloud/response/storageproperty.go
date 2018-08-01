// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// StorageProperty are storage properties used to describe the characteristics
// of storage pools and are used to determine volume placement within a pool
// when a volume is created.
// You must specify a storage property while creating a storage volume.
// For storage volumes that require low latency and high IOPS, such as for
// storing database files, select the /oracle/public/storage/latency
// storage property. For all other storage volumes,
// select /oracle/public/storage/default.
type StorageProperty struct {

	// Description of this property.
	Description string `json:"description,omitempty"`
	// Name is the storage property name
	// one of the following storage properties:
	// oracle/public/storage/default
	// oracle/public/storage/latency
	Name string `json:"name"`
	// Uri is the Uniform Resource Identifier

	Uri string `json:"uri"`
}

// AllStorageProperties
type AllStorageProperties struct {
	Result []StorageProperty `json:"result,omitempty"`
}
