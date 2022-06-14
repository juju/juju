// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"fmt"

	"github.com/juju/collections/set"
)

const (
	// K8sCloudMicrok8s is the name used for microk8s k8s clouds.
	K8sCloudMicrok8s = "microk8s"

	// K8sCloudGCE is the name used for GCE k8s clouds(GKE, CDK).
	K8sCloudGCE = "gce"

	// K8sCloudAzure is the name used for Azure k8s clouds(AKS, CDK).
	K8sCloudAzure = "azure"

	// K8sCloudEC2 is the name used for AWS k8s clouds(EKS, CDK).
	K8sCloudEC2 = "ec2"

	// K8sCloudOpenStack is the name used for openstack k8s clouds(CDK).
	K8sCloudOpenStack = "openstack"

	// K8sCloudMAAS is the name used for MAAS k8s clouds(CDK).
	K8sCloudMAAS = "maas"

	// K8sCloudLXD is the name used for LXD k8s clouds(Kubernetes Core).
	K8sCloudLXD = "lxd"

	// K8sCloudOther is the name used for any other k8s cloud is not listed above.
	K8sCloudOther = "other"

	// Microk8sRegion is the single microk8s cloud region.
	Microk8sRegion = "localhost"

	// MicroK8sClusterName is the cluster named used by microk8s.
	MicroK8sClusterName = "microk8s-cluster"
)

// ClusterMetadataChecker provides an API to query cluster metadata.
type ClusterMetadataChecker interface {
	// GetClusterMetadata returns metadata about host cloud and storage for the cluster.
	GetClusterMetadata(storageClass string) (result *ClusterMetadata, err error)

	// CheckDefaultWorkloadStorage returns an error if the opinionated storage defined for
	// the cluster does not match the specified storage.
	CheckDefaultWorkloadStorage(cluster string, storageProvisioner *StorageProvisioner) error

	// EnsureStorageProvisioner creates a storage provisioner with the specified config, or returns an existing one.
	EnsureStorageProvisioner(cfg StorageProvisioner) (*StorageProvisioner, bool, error)
}

// ClusterMetadata defines metadata about a cluster.
type ClusterMetadata struct {
	NominatedStorageClass *StorageProvisioner
	OperatorStorageClass  *StorageProvisioner
	Cloud                 string
	Regions               set.Strings
}

// PreferredStorage defines preferred storage
// attributes on a given cluster.
type PreferredStorage struct {
	Name              string
	Provisioner       string
	Parameters        map[string]string
	SupportsDefault   bool
	VolumeBindingMode string
}

// StorageProvisioner defines the a storage provisioner available on a cluster.
type StorageProvisioner struct {
	Name              string
	Provisioner       string
	Parameters        map[string]string
	Namespace         string
	Model             string
	ReclaimPolicy     string
	VolumeBindingMode string
	IsDefault         bool
}

// NonPreferredStorageError is raised when a cluster does not have
// the opinionated default storage Juju requires.
type NonPreferredStorageError struct {
	PreferredStorage
}

// Error implements error.
func (e *NonPreferredStorageError) Error() string {
	return fmt.Sprintf("preferred storage %q not available", e.Provisioner)
}

// IsNonPreferredStorageError returns true if err is a NonPreferredStorageError.
func IsNonPreferredStorageError(err error) bool {
	_, ok := err.(*NonPreferredStorageError)
	return ok
}
