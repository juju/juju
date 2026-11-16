// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"strings"
)

// storagePoolNameMaxLength mirrors the maximum length of a Juju storage
// pool name, [domain/storage.StoragePoolNameMaxLength]. The value cannot
// be imported directly (the domain package depends on this one) and is
// pinned by test to fail loudly on drift.
const storagePoolNameMaxLength = 128

// Kubernetes models are seeded with one storage pool per Kubernetes
// storage provider, named after the provider. The values cannot be
// imported directly (the provider constants package depends on this
// one) and are pinned by test to fail loudly on drift.
const (
	// kubernetesPoolName is the name of the pool seeded for the
	// Kubernetes storage provider.
	kubernetesPoolName = "kubernetes"

	// rootfsPoolName is the name of the pool seeded for the rootfs
	// storage provider.
	rootfsPoolName = "rootfs"

	// tmpfsPoolName is the name of the pool seeded for the tmpfs
	// storage provider.
	tmpfsPoolName = "tmpfs"
)

// PoolNameForStorageClass derives a valid Juju storage pool name from a
// Kubernetes storage class name. Kubernetes class names allow dots,
// leading digits and lengths beyond the Juju pool name limit, so only
// the derived name is affected: the storage-class attribute of the pool
// always records the class name verbatim.
//
// Names that would collide with one of the pools seeded in every
// Kubernetes model are prefixed, so the derived pool never shadows a
// seeded one. An empty string is returned if the class name is empty.
func PoolNameForStorageClass(className string) string {
	if className == "" {
		return ""
	}
	poolName := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, className)
	poolName = strings.Trim(poolName, "-")
	if poolName == "" || poolName[0] >= '0' && poolName[0] <= '9' {
		poolName = "k8s-" + poolName
	}
	if len(poolName) > storagePoolNameMaxLength {
		poolName = poolName[:storagePoolNameMaxLength]
	}
	poolName = strings.TrimRight(poolName, "-")
	if poolName == kubernetesPoolName ||
		poolName == rootfsPoolName ||
		poolName == tmpfsPoolName {
		poolName = "k8s-" + poolName
	}
	return poolName
}

// KubernetesFilesystemParams is a fully specified set of parameters for filesystem creation,
// derived from one or more of user-specified storage directives, a
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
	Attributes map[string]any

	// ResourceTags is a set of tags to set on the created filesystem, if the
	// storage provider supports tags.
	ResourceTags map[string]string

	// Attachments identifies the set of mount points the filesystem should be
	// mounted at.
	Attachments []KubernetesFilesystemAttachmentParams
}

// KubernetesFilesystemAttachmentParams is a set of parameters for filesystem attachment
// or detachment.
type KubernetesFilesystemAttachmentParams struct {
	// ReadOnly indicates that the storage should be attached as read-only.
	ReadOnly bool

	// Path is the path at which the filesystem is to be mounted on the pod that
	// this attachment corresponds to.
	Path string

	// ContainerName is the identifier of the container where this attachment
	// should be mounted to.
	ContainerName string

	// ProvisionedPVCNames is the list of PersistentVolumeClaim names that have been
	// created for this attachment params.
	ProvisionedPVCNames []string
}

// KubernetesFilesystemInfo describes a filesystem attachment.
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
	// UnitName is the name of the unit which the volume is attached to.
	UnitName string

	// VolumeId is the storage provider's unique identifier for the volume.
	VolumeId string
}
