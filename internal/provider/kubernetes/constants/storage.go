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

	// WorkloadStorageKey was the 3.x cloud config attribute used to record
	// the name of the storage class used to provision workload storage for
	// a Kubernetes cloud, including the persistent volume claim backing
	// the controller. It is removed in 4.0: nothing records or consumes
	// it. The key remains defined (and is registered in
	// environs/bootstrap as a bootstrap-only attribute) so 3.x cloud
	// definitions are routed and ignored with a warning rather than
	// leaking into model config.
	WorkloadStorageKey = "workload-storage"

	// OperatorStorageKey was the 3.x cloud config attribute used to record
	// the storage class for operator (podspec) storage. Operator storage is
	// no longer supported, and nothing in this package consumes the key:
	// it is documented here alongside WorkloadStorageKey as the historical
	// Kubernetes storage attribute. The routing of legacy cloud
	// definitions away from model config is done by the same value
	// registered in environs/bootstrap.
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
