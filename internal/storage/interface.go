// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/instance"
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

// ProviderRegistry is an interface for obtaining storage providers.
type ProviderRegistry interface {
	// StorageProviderTypes returns the storage provider types
	// contained within this registry.
	//
	// Determining the supported storage providers may be dynamic.
	// Multiple calls for the same registry must return consistent
	// results.
	StorageProviderTypes() ([]ProviderType, error)

	// StorageProvider returns the storage provider with the given
	// provider type. StorageProvider must return an errors satisfying
	// errors.IsNotFound if the registry does not contain the
	// specified provider type.
	StorageProvider(ProviderType) (Provider, error)
}

// Provider is an interface for obtaining storage sources.
type Provider interface {
	// VolumeSource returns a VolumeSource given the specified storage
	// provider configurations, or an error if the provider does not
	// support creating volumes or the configuration is invalid.
	//
	// If the storage provider does not support creating volumes as a
	// first-class primitive, then VolumeSource must return an error
	// satisfying errors.IsNotSupported.
	VolumeSource(*Config) (VolumeSource, error)

	// FilesystemSource returns a FilesystemSource given the specified
	// storage provider configurations, or an error if the provider does
	// not support creating filesystems or the configuration is invalid.
	FilesystemSource(*Config) (FilesystemSource, error)

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

	// Releasable reports whether or not the storage provider is capable
	// of releasing dynamic storage, with either ReleaseVolumes or
	// ReleaseFilesystems.
	Releasable() bool

	// DefaultPools returns the default storage pools for this provider,
	// to register in each new model.
	DefaultPools() []*Config

	// ValidateConfig validates the provided storage provider config,
	// returning an error if it is invalid.
	ValidateConfig(*Config) error

	// ValidateForK8s validates if a storage provider can be set for
	// a given K8s configuration.
	ValidateForK8s(map[string]any) error
}

// VolumeSource provides an interface for creating, destroying, describing,
// attaching and detaching volumes in the environment. A VolumeSource is
// configured in a particular way, and corresponds to a storage "pool".
type VolumeSource interface {
	// CreateVolumes creates volumes with the specified parameters. If the
	// volumes are initially attached, then CreateVolumes returns
	// information about those attachments too.
	CreateVolumes(ctx context.Context, params []VolumeParams) ([]CreateVolumesResult, error)

	// ListVolumes lists the provider volume IDs for every volume
	// created by this volume source.
	ListVolumes(ctx context.Context) ([]string, error)

	// DescribeVolumes returns the properties of the volumes with the
	// specified provider volume IDs.
	DescribeVolumes(ctx context.Context, volIds []string) ([]DescribeVolumesResult, error)

	// DestroyVolumes destroys the volumes with the specified provider
	// volume IDs.
	DestroyVolumes(ctx context.Context, volIds []string) ([]error, error)

	// ReleaseVolumes releases the volumes with the specified provider
	// volume IDs from the model/controller.
	ReleaseVolumes(ctx context.Context, volIds []string) ([]error, error)

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
	AttachVolumes(ctx context.Context, params []VolumeAttachmentParams) ([]AttachVolumesResult, error)

	// DetachVolumes detaches the volumes with the specified provider
	// volume IDs from the instances with the corresponding index.
	//
	// TODO(axw) we need to record in state whether or not volumes
	// are detachable, and reject attempts to attach/detach on
	// that basis.
	DetachVolumes(ctx context.Context, params []VolumeAttachmentParams) ([]error, error)
}

// FilesystemSource provides an interface for creating, destroying and
// describing filesystems in the environment. A FilesystemSource is
// configured in a particular way, and corresponds to a storage "pool".
type FilesystemSource interface {
	// ValidateFilesystemParams validates the provided filesystem creation
	// parameters, returning an error if they are invalid.
	ValidateFilesystemParams(params FilesystemParams) error

	// CreateFilesystems creates filesystems with the specified size, in MiB.
	CreateFilesystems(ctx context.Context, params []FilesystemParams) ([]CreateFilesystemsResult, error)

	// DestroyFilesystems destroys the filesystems with the specified
	// providerd filesystem IDs.
	DestroyFilesystems(ctx context.Context, fsIds []string) ([]error, error)

	// ReleaseFilesystems releases the filesystems with the specified provider
	// filesystem IDs from the model/controller.
	ReleaseFilesystems(ctx context.Context, volIds []string) ([]error, error)

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
	AttachFilesystems(ctx context.Context, params []FilesystemAttachmentParams) ([]AttachFilesystemsResult, error)

	// DetachFilesystems detaches the filesystems with the specified
	// provider filesystem IDs from the instances with the corresponding
	// index.
	DetachFilesystems(ctx context.Context, params []FilesystemAttachmentParams) ([]error, error)
}

// FilesystemImporter provides an interface for importing filesystems
// into the controller/model.
//
// TODO(axw) make this part of FilesystemSource?
type FilesystemImporter interface {
	// ImportFilesystem updates the filesystem with the specified
	// filesystem provider ID with the given resource tags, so that
	// it is seen as being managed by this Juju controller/model.
	// ImportFilesystem returns the filesystem information to store
	// in the model.
	//
	// Implementations of ImportFilesystem should validate that the
	// filesystem is not in use before allowing the import to proceed.
	// Once it is imported, it is assumed to be in a detached state.
	ImportFilesystem(
		ctx context.Context,
		filesystemId string,
		resourceTags map[string]string,
	) (FilesystemInfo, error)
}

// VolumeImporter provides an interface for importing volumes
// into the controller/model.
//
// TODO(axw) make this part of VolumeSource?
type VolumeImporter interface {
	// ImportVolume updates the volume with the specified volume
	// provider ID with the given resource tags, so that it is
	// seen as being managed by this Juju controller/model.
	// ImportVolume returns the volume information to store
	// in the model.
	//
	// Implementations of ImportVolume should validate that the
	// volume is not in use before allowing the import to proceed.
	// Once it is imported, it is assumed to be in a detached state.
	ImportVolume(
		ctx context.Context,
		volumeId string,
		resourceTags map[string]string,
	) (VolumeInfo, error)
}

// VolumeParams is a fully specified set of parameters for volume creation,
// derived from one or more of user-specified storage directives, a
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
	// This is a generic tag as it's also used to hold a unit for caas storage.
	// TODO(caas)-rename to Host
	Machine names.Tag

	// InstanceId is the ID of the cloud instance that the storage should
	// be attached to. This will only be of interest to storage providers
	// that interact with the instances, such as EBS/EC2. The InstanceId
	// field will be empty if the instance is not yet provisioned.
	InstanceId instance.Id

	// ReadOnly indicates that the storage should be attached as read-only.
	ReadOnly bool
}

// FilesystemParams is a fully specified set of parameters for filesystem creation,
// derived from one or more of user-specified storage directives, a
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

	// Attachment identifies the machine that the filesystem should be attached
	// to initially, or nil if the filesystem should not be attached to any
	// machine.
	Attachment *FilesystemAttachmentParams
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

// CreateFilesystemsResult contains the result of a FilesystemSource.CreateFilesystems call
// for one filesystem. Filesystem should only be used if Error is nil.
type CreateFilesystemsResult struct {
	Filesystem *Filesystem
	Error      error
}

// AttachFilesystemsResult contains the result of a FilesystemSource.AttachFilesystems call
// for one filesystem. FilesystemAttachment should only be used if Error is nil.
type AttachFilesystemsResult struct {
	FilesystemAttachment *FilesystemAttachment
	Error                error
}
