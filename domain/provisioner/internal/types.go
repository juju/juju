// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
)

// ProvisioningInfoState holds the raw data gathered from the model DB
// in a single transaction. The service transforms this into the final
// ProvisioningInfo.
type ProvisioningInfoState struct {
	// MachineUUID is the unique identifier of the machine.
	MachineUUID coremachine.UUID

	// Base is the OS base for the machine.
	Base corebase.Base

	// PlacementDirective is the placement directive for the machine.
	PlacementDirective *string

	// Constraints are the constraints for the machine.
	Constraints constraints.Value

	// IsController indicates whether the machine is a controller machine.
	IsController bool

	// ModelName is the name of the model this machine belongs to.
	ModelName string

	// UnitNames holds the unit names assigned to this machine with
	// their principal info.
	UnitNames []coreunit.NameWithPrincipal

	// EndpointBindings maps app name -> endpoint name -> space UUID.
	EndpointBindings map[string]map[string]network.SpaceUUID

	// VolumeParams holds storage volume provisioning parameters.
	VolumeParams []VolumeProvisioningParams

	// VolumeAttachmentParams holds storage volume attachment parameters.
	VolumeAttachmentParams []VolumeAttachmentProvisioningParams

	// RootDiskStoragePool holds the storage pool for the root disk,
	// or nil if no root-disk-source constraint was specified.
	RootDiskStoragePool *StoragePool

	// Spaces holds all spaces with their subnets and availability zones.
	Spaces network.SpaceInfos

	// CloudInitUserData holds the raw cloud-init user data YAML string
	// from model config.
	CloudInitUserData string

	// ImageStream is the image stream from model config (e.g. "released").
	ImageStream string

	// ResourceTags holds the raw resource tags string from model config
	// (space-separated key=value pairs).
	ResourceTags string

	// CloudType is the cloud type (e.g. "ec2", "azure", "openstack").
	CloudType string

	// CloudRegion is the cloud region name.
	CloudRegion string

	// CloudName is the name of the cloud (used for endpoint lookup).
	CloudName string
}

// VolumeProvisioningParams holds raw volume provisioning data from the
// state layer.
type VolumeProvisioningParams struct {
	// UUID is the unique uuid of the volume.
	UUID string

	// ID is the unique id given to the volume in the controller.
	ID string

	// Provider is the storage provider name.
	Provider string

	// RequestedSizeMiB is the requested minimum size.
	RequestedSizeMiB uint64

	// Attributes holds provider-specific attributes.
	Attributes map[string]string

	// StorageName is the name of the storage instance (e.g. "data").
	StorageName string

	// StorageID is the ID of the storage instance (numeric string).
	StorageID string

	// StorageOwnerUnitName is the name of the unit that owns the storage
	// (nil if no owner).
	StorageOwnerUnitName *string
}

// VolumeAttachmentProvisioningParams holds raw volume attachment data
// from the state layer.
type VolumeAttachmentProvisioningParams struct {
	// VolumeUUID is the UUID of the volume this attachment is for.
	VolumeUUID string

	// VolumeID is the unique ID of the volume.
	VolumeID string

	// Provider is the storage provider name.
	Provider string

	// ReadOnly indicates the volume should be attached read-only.
	ReadOnly bool

	// VolumeProviderID is the provider-assigned ID.
	VolumeProviderID string
}

// StoragePool holds storage pool information.
type StoragePool struct {
	// Provider is the storage provider name.
	Provider string

	// Attrs holds provider-specific pool attributes.
	Attrs map[string]string
}

// CloudImageMetadata holds image metadata for provisioning.
type CloudImageMetadata struct {
	// ImageID is the provider image identifier.
	ImageID string

	// Stream is the image stream (e.g. "released", "daily").
	Stream string

	// Region is the cloud region.
	Region string

	// Version is the OS version.
	Version string

	// Arch is the architecture.
	Arch string

	// VirtType is the virtualisation type.
	VirtType string

	// RootStorageType is the root storage type.
	RootStorageType string

	// RootStorageSize is the root storage size in MB (optional).
	RootStorageSize *uint64

	// Source indicates where the metadata came from.
	Source string

	// Priority is the priority of the metadata source.
	Priority int
}

// ImageConstraint holds the parameters for an image metadata search.
type ImageConstraint struct {
	// Releases holds the OS release versions to match.
	Releases []string

	// Arches holds the architectures to match.
	Arches []string

	// Stream is the image stream to search.
	Stream string

	// Region is the cloud region to match.
	Region string

	// Endpoint is the cloud endpoint URL.
	Endpoint string

	// ImageID is a specific image ID to match (optional).
	ImageID *string
}
