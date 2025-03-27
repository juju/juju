// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"github.com/juju/collections/set"
	storagev1 "k8s.io/api/storage/v1"
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
}

// ClusterMetadata defines metadata about a cluster.
type ClusterMetadata struct {
	WorkloadStorageClass *storagev1.StorageClass
	OperatorStorageClass *storagev1.StorageClass
	Cloud                string
	Regions              set.Strings
}

// StorageProvisioner defines the a storage provisioner available on a cluster.
type StorageProvisioner struct {
	Name              string
	Provisioner       string
	Parameters        map[string]string
	Namespace         string
	ModelName         string
	ModelUUID         string
	ControllerUUID    string
	ReclaimPolicy     string
	VolumeBindingMode string
	IsDefault         bool
}
