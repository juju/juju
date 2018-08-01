// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// StorageVolueme is a storage volume is virtual
// disk drive that provides block storage for
// Oracle Compute Cloud Service instances.
type StorageVolume struct {

	// Account is the the default account for your identity domain
	Account string `json:"account"`

	// Bootable is a a boolean field that indicates whether
	// the storage volume can be used as the boot
	// disk for an instance.
	// If you set the value to true, then you must
	// specify values for the following parameters imagelist
	// The machine image that you want to extract
	// on to the storage volume that you're creating.
	// imagelist_entry field (optional)
	// for the version of the image list entry
	// that you want to extract. The default value is 1.
	Bootable bool `json:"bootable"`

	// Description is the description of the
	// storage volume
	Description *string `json:"description"`

	// Hypervisor is the hypervisor that this volume is compatible with
	Hypervisor *string `json:"hypervisor"`

	// Imagelist is the name of machineimage
	// to extract onto this volume when created.<Paste>
	Imagelist string `json:"imagelist"`

	// Imagelist_entry is a pecific imagelist entry version to extract.
	Imagelist_entry int `json:"imagelist_entry"`

	// Machineimage_name three-part name of the machine image.
	// This information is available if the volume
	// is a bootable storage volume
	Machineimage_name string `json:"machineimage_name"`

	// Managed flag indicating that all volumes are managed volumes.
	// Default value is true.
	Managed bool `json:"managed"`

	// Name is the name of the storage volume
	Name string `json:"name"`

	// Platform is the OS platform this volume is compatible with
	Platform string `json:"platform"`

	// Properties contains the storage-pool properties
	// For storage volumes that require low latency and high IOPS,
	// such as for storing database files, specify common.LatencyPool
	// For all other storage volumes, specify common.DefaultPool
	Properties []common.StoragePool `json:"properties"`

	// Quota field not used
	Quota *string `json:"quota,omitempty"`

	// Readonly boolean field indicating whether this volume
	// can be attached as readonly. If set to False the
	// volume will be attached as read-write
	Readonly bool `json:"readonly"`

	// Shared not used
	Shared bool `json:"shared"`

	// Size is the the size of this storage volume.
	// Use one of the following abbreviations for the unit of measurement:
	// B or b for bytes
	// K or k for kilobytes
	// M or m for megabytes
	// G or g for gigabytes
	// T or t for terabytes
	// For example, to create a volume of size 10 gigabytes,
	// you can specify 10G, or 10240M, or 10485760K, and so on.
	// The allowed range is from 1 GB to 2 TB, in increments of 1 GB.
	// If you are creating a bootable storage volume, ensure
	// that the size of the storage volume is greater
	// than the size of the machine image that you want
	// to extract on to the storage volume.
	// If you are creating this storage volume from a storage snapshot,
	// ensure that the size of the storage volume that you create
	// is greater than the size of the storage snapshot.
	Size int `json:",string"`

	// Snapshot multipart name of the storage volume snapshot if
	// this storage volume is a clone.
	Snapshot *string `json:"snapshot"`

	// Snapshot_account of the parent snapshot
	// from which the storage volume is restored
	Snapshot_account string `json:"snapshot_account"`

	// Snapshot_id is the dd of the parent snapshot
	// from which the storage volume is restored or cloned
	Snapshot_id string `json:"snapshot_id"`

	// Status it the current state of the storage volume
	Status common.VolumeState `json:"status"`

	// Status_details details about the latest state of the storage volume.
	Status_detail string `json:"status_detail"`

	// Status_timestamp indicates the time that the current
	// view of the storage volume was generated
	Status_timestamp string `json:"status_timestamp"`

	// Storage_pool is the storage
	// pool from which this volume is allocated
	Storage_pool string `json:"storage_pool"`

	// Tags are strings that you can use to tag the storage volume.
	Tags []string `json:"tags,omitempty"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`

	// Writecache not used
	Writecache bool `json:"writecache"`
}

type AllStorageVolumes struct {
	Result []StorageVolume `json:"result,omitempty"`
}
