// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/life"
)

// MachineVolumeAttachmentProvisioningParams defines the set of parameters
// required for attaching a volume a machine during machine provisioning.
type MachineVolumeAttachmentProvisioningParams struct {
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

// MachineVolumeProvisioningParams defines the set of parameters required to
// provision volumes alongside machines in the environ.
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

	// Tags represents the set of tags that should be applied to the volume by
	// the storage provider.
	Tags map[string]string
}

// VolumeAttachmentID is a struct that provides the IDs and names associated
// with a volume attachment. In this case the id refers to the volume
// resource the attachment is for. As well as this the name of the machine and
// or the unit the attachment is for is also provided.
//
// As it is unclear if a volume attachment is for a unit or a machine either
// one of the name values will be set but not both.
type VolumeAttachmentID struct {
	// VolumeID is the ID of the volume resource that the attachment is
	// for.
	VolumeID string

	// MachineName is the name of the machine the volume attachment is
	// against. Only one of [VolumeAttachmentID.MachineName] or
	// [VolumeAttachmentID.UnitName] will be set. It is reasonable to expect
	// one of these values to be set.
	MachineName *coremachine.Name

	// UnitName is the name of the unit the volume attachment is against.
	// Only one of [VolumeAttachmentID.MachineName] or
	// [VolumeAttachmentID.UnitName] will be set. It is reasonable to expect
	// one of these values to be set.
	UnitName *coreunit.Name
}

// Volume is a struct that provides the information about a volume.
type Volume struct {
	// VolumeID is the ID of the volume.
	VolumeID string

	// ProviderID is the ID of the volume from the storage provider.
	ProviderID string

	// SizeMiB is the size of the volume in MiB.
	SizeMiB uint64

	// HardwareID is set by the storage provider to help matching with a block
	// device.
	HardwareID string

	// WWN is set by the storage provider to help matching with a block device.
	WWN string

	// Persistent is true if the volume is persistent.
	Persistent bool
}

// VolumeProvisionedInfo is information set by the storage provisioner for
// volumes it has provisioned.
type VolumeProvisionedInfo struct {
	// ProviderID is the ID of the volume from the storage provider.
	ProviderID string

	// SizeMiB is the size of the volume in MiB.
	SizeMiB uint64

	// HardwareID is set by the storage provider to help matching with a block
	// device.
	HardwareID string

	// WWN is set by the storage provider to help matching with a block device.
	WWN string

	// Persistent is true if the volume is persistent.
	Persistent bool
}

// VolumeAttachment is a struct that provides the information about a volume
// attachment.
type VolumeAttachment struct {
	VolumeID string

	ReadOnly bool

	BlockDeviceName       string
	BlockDeviceLinks      []string
	BlockDeviceBusAddress string
}

// VolumeAttachmentPlan is a struct that provides the information about a volume
// attachment plan.
type VolumeAttachmentPlan struct {
	Life             life.Life
	DeviceType       PlanDeviceType
	DeviceAttributes map[string]string
}

// VolumeAttachmentProvisionedInfo is information set by the storage provisioner
// for volume attachments it has provisioned.
type VolumeAttachmentProvisionedInfo struct {
	ReadOnly        bool
	BlockDeviceUUID *blockdevice.BlockDeviceUUID
}

// VolumeAttachmentPlanProvisionedInfo is information set by the storage
// provisioner for volume attachments it has provisioned.
type VolumeAttachmentPlanProvisionedInfo struct {
	DeviceType       PlanDeviceType
	DeviceAttributes map[string]string
}

// VolumeAttachmentParams defines the set of parameters that a caller needs to
// know in order to provision a volume attachment in the model.
type VolumeAttachmentParams struct {
	Machine           *coremachine.Name
	MachineInstanceID string
	Provider          string
	ProviderID        string
	ReadOnly          bool
}

// VolumeParams defines the set of parameters that a caller needs to know in
// order to provision a volume in the model.
type VolumeParams struct {
	Attributes           map[string]string
	ID                   string
	Provider             string
	SizeMiB              uint64
	VolumeAttachmentUUID *VolumeAttachmentUUID
}

// VolumeRemovalParams defines the set of parameters that a caller needs to
// know in order to de-provision a volume in the model.
type VolumeRemovalParams struct {
	// Provider is the name of the provider that is provisioning this volume.
	Provider string

	// ProviderID is the ID of the volume from the storage provider.
	ProviderID string

	// Obliterate is true when the provisioner should remove the volume from
	// existence.
	Obliterate bool
}
