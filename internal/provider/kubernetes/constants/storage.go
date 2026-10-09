// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

import (
	"regexp"

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

	// WorkloadStorageKey is the cloud config attribute used to record the
	// name of the storage class used to provision workload storage for a
	// Kubernetes cloud, including the persistent volume claim backing the
	// controller. It is a bootstrap-only attribute: add-k8s records it on
	// the cloud and bootstrap consumes it when creating the default
	// storage pool for the controller model.
	WorkloadStorageKey = "workload-storage"

	// OperatorStorageKey was the 3.x cloud config attribute used to record
	// the storage class for operator (podspec) storage. Operator storage is
	// no longer supported. The key is retained only so 3.x cloud definitions
	// are routed (and ignored) rather than leaking into model config.
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
	// LegacyPVNameRegexp matches how Juju labels persistent volumes.
	// The pattern is: juju-<storagename>-<digit>
	LegacyPVNameRegexp = regexp.MustCompile(`^juju-(?P<storageName>\D+)-\d+$`)

	// PVNameRegexp matches how Juju labels persistent volumes.
	// The pattern is: <storagename>-<digit>
	PVNameRegexp = regexp.MustCompile(`^(?P<storageName>\D+)-\w+$`)
)
