// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/internal/errors"
)

const (
	// K8sStorageMediumConst is the constant key
	K8sStorageMediumConst = "storage-medium"
	// K8sStorageMediumMemory is the value use tmpfs in K8s
	K8sStorageMediumMemory = "Memory"
	// K8sStorageMediumHugePages is a K8s storage for volumes
	K8sStorageMediumHugePages = "HugePages"
)

const (
	// StorageClass is the name of a storage class resource.
	K8sStorageClass       = "storage-class"
	K8sStorageProvisioner = "storage-provisioner"
	K8sStorageMedium      = "storage-medium"
	K8sStorageMode        = "storage-mode"
)

const (
	// FilesystemCreateParamsIncomplete is used to signal to the storage
	// provisioner that it needs to wait for more filesystem create parameters
	// to change before trying to create the filesystem.
	FilesystemCreateParamsIncomplete = errors.ConstError("filesystem create params incomplete")

	// FilesystemAttachParamsIncomplete is used to signal to the storage
	// provisioner that it needs to wait for more filesystem attach parameters
	// to change before trying to attach the filesystem.
	FilesystemAttachParamsIncomplete = errors.ConstError("filesystem attach params incomplete")
)
