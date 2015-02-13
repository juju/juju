// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/names"
)

// ProviderType uniquely identifies a storage provider, such as "ebs" or "loop".
type ProviderType string

// Provider is an interface for obtaining storage sources.
type Provider interface {
	// VolumeSource returns a VolumeSource given the
	// specified cloud and storage provider configurations.
	//
	// If the storage provider does not support creating volumes as a
	// first-class primitive, then VolumeSource must return an error
	// satisfying errors.IsNotSupported.
	VolumeSource(environConfig *config.Config, providerConfig *Config) (VolumeSource, error)

	// TODO(axw) define filesystem source. If the user requests a
	// filesystem and that can be provided first-class, it should be
	// done that way. Otherwise we create a volume and then manage a
	// filesystem on that.

	// ValidateConfig validates the provided storage provider config,
	// returning an error if it is invalid.
	ValidateConfig(*Config) error
}

// VolumeSource provides an interface for creating, destroying, describing,
// attaching and detaching volumes in the environment. A VolumeSource is
// configured in a particular way, and corresponds to a storage "pool".
type VolumeSource interface {
	// CreateVolumes creates volumes with the specified parameters. If the
	// volumes are initially attached, then CreateVolumes returns
	// information about those attachments too.
	CreateVolumes(params []VolumeParams) ([]Volume, []VolumeAttachment, error)

	// DescribeVolumes returns the properties of the volumes with the
	// specified provider volume IDs.
	DescribeVolumes(volIds []string) ([]Volume, error)

	// DestroyVolumes destroys the volumes with the specified provider
	// volume IDs.
	DestroyVolumes(volIds []string) error

	// ValidateVolumeParams validates the provided volume creation
	// parameters, returning an error if they are invalid.
	//
	// If the provider is incapable of provisioning volumes separately
	// from machine instances (e.g. MAAS), then ValidateVolumeParams
	// must return an error if params.Instance is non-empty.
	ValidateVolumeParams(params VolumeParams) error

	// AttachVolumes attaches the volumes with the specified provider
	// volume IDs to the instances with the corresponding index.
	//
	// TODO(axw) we need to validate attachment requests prior to
	// recording in state. For example, the ec2 provider must reject
	// an attempt to attach a volume to an instance if they are in
	// different availability zones.
	AttachVolumes(params []VolumeAttachmentParams) ([]VolumeAttachment, error)

	// DetachVolumes detaches the volumes with the specified provider
	// volume IDs from the instances with the corresponding index.
	//
	// TODO(axw) we need to record in state whether or not volumes
	// are detachable, and reject attempts to attach/detach on
	// that basis.
	DetachVolumes(params []VolumeAttachmentParams) error
}

// VolumeParams is a fully specified set of parameters for volume creation,
// derived from one or more of user-specified storage constraints, a
// storage pool definition, and charm storage metadata.
type VolumeParams struct {
	// Tag is a unique tag name assigned by Juju for the requested volume.
	Tag names.DiskTag

	// Size is the minimum size of the volume in MiB.
	Size uint64

	// Provider is the name of the storage provider that is to be used to
	// create the volume.
	Provider ProviderType

	// Attributes is the set of provider-specific attributes to pass to
	// the storage provider when creating the volume.
	Attributes map[string]interface{}

	// Attachment identifies the machine that the volume should be attached
	// to initially, or nil if the volume should not be attached to any
	// machine. Some providers, such as MAAS, do not support dynamic
	// attachment, and so provisioning time is the only opportunity to
	// perform attachment.
	//
	// When machine instances are created, the instance provider will be
	// presented with parameters for any due-to-be-attached volumes. If
	// once the instance is created there are still unprovisioned volumes,
	// the dynamic storage provisioner will take care of creating them.
	Attachment *AttachmentParams
}

// VolumeAttachmentParams is a set of parameters for volume attachment or
// detachment.
type VolumeAttachmentParams struct {
	AttachmentParams

	// Volume is a unique tag assigned by Juju for the volume that
	// should be attached/detached.
	Volume names.DiskTag

	// VolumeId is the unique provider-supplied ID for the volume that
	// should be attached/detached.
	VolumeId string
}

// AttachmentParams describes the parameters for attaching a volume or
// filesystem to a machine.
type AttachmentParams struct {
	// Machine is the tag of the Juju machine that the storage should be
	// attached to. Storage providers may use this to perform machine-
	// specific operations, such as configuring access controls for the
	// machine.
	Machine names.MachineTag

	// InstanceId is the ID of the cloud instance that the storage should
	// be attached to. This will only be of interest to storage providers
	// that interact with the instances, such as EBS/EC2. The InstanceId
	// field will be empty if the instance is not yet provisioned.
	InstanceId instance.Id
}
