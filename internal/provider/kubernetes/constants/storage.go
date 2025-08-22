// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

import (
	"regexp"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/storage"
)

const (
	// StorageProviderTypeRootfs defines the Juju storage type for rootfs
	// storage provisioning in Kuberntes.
	StorageProviderTypeRootfs = storage.ProviderType("rootfs")

	// StorageProviderTypeTmpfs defines the Juju storage type for tmpfs storage
	// provisioning in Kuberntes.
	StorageProviderTypeTmpfs = storage.ProviderType("tmpfs")

	// StorageProviderType defines the Juju storage type which can be used
	// to provision storage on k8s models.
	StorageProviderType = storage.ProviderType("kubernetes")

	// K8s storage pool attributes.

	// StorageClass is the name of a storage class resource.
	StorageClass       = "storage-class"
	StorageProvisioner = "storage-provisioner"
	StorageMedium      = "storage-medium"
	StorageMode        = "storage-mode"
)

// QualifiedStorageClassName returns a qualified storage class name.
func QualifiedStorageClassName(namespace, storageClass string) string {
	if namespace == "" {
		return storageClass
	}
	return namespace + "-" + storageClass
}

var (
	// StorageBaseDir is the base storage dir for the k8s series.
	StorageBaseDir = getK8sStorageBaseDir()

	// LegacyPVNameRegexp matches how Juju labels persistent volumes.
	// The pattern is: juju-<storagename>-<digit>
	LegacyPVNameRegexp = regexp.MustCompile(`^juju-(?P<storageName>\D+)-\d+$`)

	// PVNameRegexp matches how Juju labels persistent volumes.
	// The pattern is: <storagename>-<digit>
	PVNameRegexp = regexp.MustCompile(`^(?P<storageName>\D+)-\w+$`)
)

func getK8sStorageBaseDir() string {
	return paths.StorageDir(paths.OSUnixLike)
}
