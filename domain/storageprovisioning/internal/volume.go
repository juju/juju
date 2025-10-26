// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
)

// MachineVolumeAttachmentProvisioningParams is a internal type for representing
// the machine provisioning parameters for volume attachments from state.
type MachineVolumeAttachmentProvisioningParams struct {
	// BlockDeviceUUID is the id of the connected block device on the attached
	// entity. If no block device association exists this value will be nil.
	BlockDeviceUUID *domainblockdevice.BlockDeviceUUID

	// Provider is the storage provider to use when provisioning the volume.
	Provider string

	// ReadOnly indicates if the volume should be attached to the machine as
	// read only.
	ReadOnly bool

	// VolumeID is the unique id given to the volume in the controller. This is
	// not the volume uuid.
	VolumeID string

	// VolumeProviderID is the unique id given to the volume by the storage
	// provider. This value is opaque to Juju.
	VolumeProviderID string
}

// MachineVolumeProvisioningParams is a internal type for representing the
// machine provisioning paramters for a volume from state.
type MachineVolumeProvisioningParams struct {
	// Attributes is the set of provider specific attributes to use when
	// provisioning and managing the volume.
	Attributes map[string]string

	// ID is the unique id given to the volume in the controller. This is not
	// the volume uuid.
	ID string

	// Provider is the storage provider to use when provisioning the volume.
	Provider string

	// RequestedSizeMiB is the requested size the volume should be at least
	// provisioned as. See [MachineVolumeProvisioningParams.SizeMiB] for the
	// actual size of the volume once provisioned.
	RequestedSizeMiB uint64

	// SizeMiB is the actual provisioned sized of the volume. See
	// [MachineVolumeProvisioningParams.RequestedSizeMiB] for the requested size
	// when being provisioned. A value of 0 here means the volume has not been
	// provisioned yet and the final size is not known.
	SizeMiB uint64

	// StorageID is the id associated with the storage instance this volume
	// fulfills. Not to be confused with the storage instance's uuid.
	StorageID string

	// StorageOwnerUnitName is the name of the unit the associated storage
	// instance is attached to. This value will only ever be set if the storage
	// instance is currently attached to a unit and the storage instance is not
	// shared.
	StorageOwnerUnitName *string
}
