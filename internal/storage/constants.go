// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

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
