// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/names"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
)

// ProviderType uniquely identifies a storage provider, such as "ebs" or "loop".
type ProviderType string

// Scope defines the scope of the storage that a provider manages.
// Machine-scoped storage must be managed from within the machine,
// whereas environment-level storage must be managed by an environment
// storage provisioner.
type Scope int

const (
	ScopeEnviron Scope = iota
	ScopeMachine
)

// Provider is an interface for obtaining storage sources.
type Provider interface {
	// VolumeSource returns a VolumeSource given the specified cloud
	// and storage provider configurations, or an error if the provider
	// does not support creating volumes or the configuration is invalid.
	//
	// If the storage provider does not support creating volumes as a
	// first-class primitive, then VolumeSource must return an error
	// satisfying errors.IsNotSupported.
	VolumeSource(environConfig *config.Config, providerConfig *Config) (VolumeSource, error)

	// FilesystemSource returns a FilesystemSource given the specified
	// cloud and storage provider configurations, or an error if the
	// provider does not support creating filesystems or the configuration
	// is invalid.
	FilesystemSource(environConfig *config.Config, providerConfig *Config) (FilesystemSource, error)

	// Supports reports whether or not the storage provider supports
	// the specified storage kind.
	//
	// A provider that supports volumes but not filesystems can still
	// be used for creating filesystem storage; Juju will request a
	// volume from the provider and then manage the filesystem itself.
	Supports(kind StorageKind) bool

	// Scope returns the scope of storage managed by this provider.
	Scope() Scope

	// Dynamic reports whether or not the storage provider is capable
	// of dynamic storage provisioning. Non-dynamic storage must be
	// created at the time a machine is provisioned.
	Dynamic() bool

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
	CreateVolumes(params []VolumeParams) ([]CreateVolumesResult, error)

	// ListVolumes lists the provider volume IDs for every volume
	// created by this volume source.
	ListVolumes() ([]string, error)

	// DescribeVolumes returns the properties of the volumes with the
	// specified provider volume IDs.
	DescribeVolumes(volIds []string) ([]DescribeVolumesResult, error)

	// DestroyVolumes destroys the volumes with the specified provider
	// volume IDs.
	DestroyVolumes(volIds []string) ([]error, error)

	// ValidateVolumeParams validates the provided volume creation
	// parameters, returning an error if they are invalid.
	ValidateVolumeParams(params VolumeParams) error

	// AttachVolumes attaches volumes to machines.
	//
	// AttachVolumes must be idempotent; it may be called even if the
	// attachment already exists, to ensure that it exists, e.g. over
	// machine restarts.
	//
	// TODO(axw) we need to validate attachment requests prior to
	// recording in state. For example, the ec2 provider must reject
	// an attempt to attach a volume to an instance if they are in
	// different availability zones.
	AttachVolumes(params []VolumeAttachmentParams) ([]AttachVolumesResult, error)

	// DetachVolumes detaches the volumes with the specified provider
	// volume IDs from the instances with the corresponding index.
	//
	// TODO(axw) we need to record in state whether or not volumes
	// are detachable, and reject attempts to attach/detach on
	// that basis.
	DetachVolumes(params []VolumeAttachmentParams) ([]error, error)
}

// FilesystemSource provides an interface for creating, destroying and
// describing filesystems in the environment. A FilesystemSource is
// configured in a particular way, and corresponds to a storage "pool".
type FilesystemSource interface {
	// ValidateFilesystemParams validates the provided filesystem creation
	// parameters, returning an error if they are invalid.
	ValidateFilesystemParams(params FilesystemParams) error

	// CreateFilesystems creates filesystems with the specified size, in MiB.
	CreateFilesystems(params []FilesystemParams) ([]Filesystem, error)

	// DestroyFilesystems destroys the filesystems with the specified
	// providerd filesystem IDs.
	DestroyFilesystems(fsIds []string) []error

	// AttachFilesystems attaches filesystems to machines.
	//
	// AttachFilesystems must be idempotent; it may be called even if
	// the attachment already exists, to ensure that it exists, e.g. over
	// machine restarts.
	//
	// TODO(axw) we need to validate attachment requests prior to
	// recording in state. For example, the ec2 provider must reject
	// an attempt to attach a volume to an instance if they are in
	// different availability zones.
	AttachFilesystems(params []FilesystemAttachmentParams) ([]FilesystemAttachment, error)

	// DetachFilesystems detaches the filesystems with the specified
	// provider filesystem IDs from the instances with the corresponding
	// index.
	DetachFilesystems(params []FilesystemAttachmentParams) error
}

// VolumeParams is a fully specified set of parameters for volume creation,
// derived from one or more of user-specified storage constraints, a
// storage pool definition, and charm storage metadata.
type VolumeParams struct {
	// Tag is a unique tag name assigned by Juju for the requested volume.
	Tag names.VolumeTag

	// Size is the minimum size of the volume in MiB.
	Size uint64

	// Provider is the name of the storage provider that is to be used to
	// create the volume.
	Provider ProviderType

	// Attributes is the set of provider-specific attributes to pass to
	// the storage provider when creating the volume. Attributes is derived
	// from the storage pool configuration.
	Attributes map[string]interface{}

	// ResourceTags is a set of tags to set on the created volume, if the
	// storage provider supports tags.
	ResourceTags map[string]string

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
	Attachment *VolumeAttachmentParams
}

// IsPersistent returns true if the params has persistent set to true.
func (p *VolumeParams) IsPersistent() bool {
	v, _ := p.Attributes[Persistent].(bool)
	return v
}

// VolumeAttachmentParams is a set of parameters for volume attachment or
// detachment.
type VolumeAttachmentParams struct {
	AttachmentParams

	// Volume is a unique tag assigned by Juju for the volume that
	// should be attached/detached.
	Volume names.VolumeTag

	// VolumeId is the unique provider-supplied ID for the volume that
	// should be attached/detached.
	VolumeId string
}

// AttachmentParams describes the parameters for attaching a volume or
// filesystem to a machine.
type AttachmentParams struct {
	// Provider is the name of the storage provider that is to be used to
	// create the attachment.
	Provider ProviderType

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

	// ReadOnly indicates that the storage should be attached as read-only.
	ReadOnly bool
}

// FilesystemParams is a fully specified set of parameters for filesystem creation,
// derived from one or more of user-specified storage constraints, a
// storage pool definition, and charm storage metadata.
type FilesystemParams struct {
	// Tag is a unique tag assigned by Juju for the requested filesystem.
	Tag names.FilesystemTag

	// Volume is the tag of the volume that backs the filesystem, if any.
	Volume names.VolumeTag

	// Size is the minimum size of the filesystem in MiB.
	Size uint64

	// The provider type for this filesystem.
	Provider ProviderType

	// Attributes is a set of provider-specific options for storage creation,
	// as defined in a storage pool.
	Attributes map[string]interface{}

	// ResourceTags is a set of tags to set on the created filesystem, if the
	// storage provider supports tags.
	ResourceTags map[string]string
}

// FilesystemAttachmentParams is a set of parameters for filesystem attachment
// or detachment.
type FilesystemAttachmentParams struct {
	AttachmentParams

	// Filesystem is a unique tag assigned by Juju for the filesystem that
	// should be attached/detached.
	Filesystem names.FilesystemTag

	// FilesystemId is the unique provider-supplied ID for the filesystem that
	// should be attached/detached.
	FilesystemId string

	// Path is the path at which the filesystem is to be mounted on the machine that
	// this attachment corresponds to.
	Path string
}

// CreateVolumesResult contains the result of a VolumeSource.CreateVolumes call
// for one volume. Volume and VolumeAttachment should only be used if Error is
// nil.
type CreateVolumesResult struct {
	Volume           *Volume
	VolumeAttachment *VolumeAttachment
	Error            error
}

// DescribeVolumesResult contains the result of a VolumeSource.DescribeVolumes call
// for one volume. Volume should only be used if Error is nil.
type DescribeVolumesResult struct {
	VolumeInfo *VolumeInfo
	Error      error
}

// AttachVolumesResult contains the result of a VolumeSource.AttachVolumes call
// for one volume. VolumeAttachment should only be used if Error is nil.
type AttachVolumesResult struct {
	VolumeAttachment *VolumeAttachment
	Error            error
}
