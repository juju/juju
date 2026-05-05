// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/provisioner/internal"
)

// ProvisioningInfoState is the raw data gathered from the model DB in a
// single transaction. The service transforms this into ProvisioningInfo.
type ProvisioningInfoState = internal.ProvisioningInfoState

// VolumeProvisioningParams holds raw volume provisioning data from the
// state layer.
type VolumeProvisioningParams = internal.VolumeProvisioningParams

// VolumeAttachmentProvisioningParams holds raw volume attachment data
// from the state layer.
type VolumeAttachmentProvisioningParams = internal.VolumeAttachmentProvisioningParams

// StoragePool holds storage pool information.
type StoragePool = internal.StoragePool

// CloudImageMetadata holds image metadata for provisioning.
type CloudImageMetadata = internal.CloudImageMetadata

// ImageConstraint holds the parameters for an image metadata search.
type ImageConstraint = internal.ImageConstraint

// ProvisioningInfo holds the complete set of information required to
// provision a machine instance. This is the final output of the
// provisioning service -- ready to be mapped to API params by the facade.
type ProvisioningInfo struct {
	// MachineUUID is the unique identifier of the machine.
	MachineUUID coremachine.UUID

	// Base is the OS base for the machine.
	Base corebase.Base

	// PlacementDirective is the placement directive for the machine, or nil
	// if no placement was specified.
	PlacementDirective *string

	// Constraints are the constraints for the machine.
	Constraints constraints.Value

	// Jobs lists the jobs this machine is responsible for.
	Jobs []model.MachineJob

	// EndpointBindings maps endpoint names to resolved space provider
	// IDs or space names (for the provider).
	EndpointBindings map[string]string

	// Volumes holds the volume provisioning parameters.
	Volumes []VolumeParams

	// VolumeAttachments holds volume attachment parameters for volumes
	// that already exist and need only to be attached.
	VolumeAttachments []VolumeAttachmentParams

	// RootDisk holds the root disk volume parameters, or nil if no
	// root-disk-source constraint was specified.
	RootDisk *VolumeParams

	// ImageMetadata holds image metadata for provisioning.
	ImageMetadata []CloudImageMetadata

	// Tags holds the instance tags to apply to the machine.
	Tags map[string]string

	// SpaceSubnets maps space names to the provider subnet IDs within
	// that space.
	SpaceSubnets map[string][]string

	// SubnetAZs maps provider subnet IDs to availability zones.
	SubnetAZs map[string][]string

	// CloudInitUserData holds cloud-init user data from model config.
	CloudInitUserData map[string]any

	// ControllerConfig holds the controller configuration.
	ControllerConfig controller.Config
}

// VolumeParams holds volume provisioning parameters.
type VolumeParams struct {
	// VolumeID is the unique ID given to the volume in the controller.
	VolumeID string

	// Provider is the storage provider name.
	Provider string

	// SizeMiB is the requested size in MiB.
	SizeMiB uint64

	// Attributes holds provider-specific attributes.
	Attributes map[string]any

	// Tags holds tags to apply to the volume.
	Tags map[string]string

	// Attachment holds the attachment parameters if this volume is
	// being created and attached simultaneously.
	Attachment *VolumeAttachmentParams
}

// VolumeAttachmentParams holds parameters for attaching a volume
// to a machine.
type VolumeAttachmentParams struct {
	// VolumeID is the unique ID of the volume to attach.
	VolumeID string

	// MachineID is the machine tag string for the attachment target.
	MachineID string

	// Provider is the storage provider name.
	Provider string

	// ReadOnly indicates the volume should be attached read-only.
	ReadOnly bool

	// ProviderID is the provider-assigned ID of the volume.
	ProviderID string
}
