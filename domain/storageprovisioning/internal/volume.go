// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
)

// MachineVolumeProvisioningParams defines the set of parameters required to
// provision volumes alongside machines in the environ.
type MachineVolumeProvisioningParams struct {
	MachineVolumeAttachmentProvisioningParams

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
}

// MachineVolumeAttachmentProvisioningParams describes the attachment for volume
// onto a machine when the volume is to be provisioned with the machine.
type MachineVolumeAttachmentProvisioningParams struct {
	// ProvisioningScope indicates the provisioning scope of the attachment.
	ProvisioningScope domainstorageprovisioning.ProvisionScope

	// ReadOnly indicates if the volume should be attached read only to the
	// machine.
	ReadOnly bool
}
