// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

import (
	"regexp"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/storage"
)

const (
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

const (
	// WorkloadStorageKey is the model config attribute used to specify
	// the storage class for provisioning workload storage.
	WorkloadStorageKey = "workload-storage"

	// OperatorStorageKey is the model config attribute used to specify
	// the storage class for provisioning operator storage.
	OperatorStorageKey = "operator-storage"
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
