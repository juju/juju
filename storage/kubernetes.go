// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// KubernetesFilesystemParams is a fully specified set of parameters for filesystem creation,
// derived from one or more of user-specified storage constraints, a
// storage pool definition, and charm storage metadata.
type KubernetesFilesystemParams struct {
	// StorageName is the name of the storage as specified in the charm.
	StorageName string

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

	// Attachment identifies the mount point the filesystem should be
	// mounted at.
	Attachment *KubernetesFilesystemAttachmentParams
}

// KubernetesFilesystemAttachmentParams is a set of parameters for filesystem attachment
// or detachment.
type KubernetesFilesystemAttachmentParams struct {
	AttachmentParams

	// Path is the path at which the filesystem is to be mounted on the pod that
	// this attachment corresponds to.
	Path string
}

// FilesystemAttachmentInfo describes a filesystem attachment.
type KubernetesFilesystemInfo struct {
	// MountPoint is the path the filesystem is mounted at.
	MountPoint string

	// ReadOnly is true if the filesystem is readonly.
	ReadOnly bool

	// FilesystemId is a unique provider id for the filesystem.
	FilesystemId string

	// Pool is the name of the storage pool used to
	// allocate the filesystem.
	Pool string

	// Size is the size of the filesystem in MiB.
	Size uint64
}

// KubernetesFilesystemUnitAttachmentParams describes a unit filesystem attachment.
type KubernetesFilesystemUnitAttachmentParams struct {
	UnitName string
	VolumeId string
}
