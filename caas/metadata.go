// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"fmt"

	"github.com/juju/collections/set"
	// core "k8s.io/api/core/v1"
)

const (
	// K8sCloudMicrok8s is the name used for microk8s k8s clouds.
	K8sCloudMicrok8s = "microk8s"
	// K8sCloudGCE is the name used for GCE k8s clouds.
	K8sCloudGCE = "gce"
	// K8sCloudAzure is the name used for Azure k8s clouds.
	K8sCloudAzure = "azure"
	// K8sCloudEC2 is the name used for AWS k8s clouds.
	K8sCloudEC2 = "ec2"
	// K8sCloudCDK is the name used for CDK k8s clouds.
	K8sCloudCDK = "cdk"

	// Microk8sRegion is the single microk8s cloud region.
	Microk8sRegion = "localhost"
)

// PreferredStorage defines preferred storage
// attributes on a given cluster.
type PreferredStorage struct {
	Name        string
	Provisioner string
	Parameters  map[string]string
}

// StorageProvisioner defines the a storage provisioner available on a cluster.
type StorageProvisioner struct {
	Name          string
	Provisioner   string
	Parameters    map[string]string
	Namespace     string
	ReclaimPolicy string
}

// ClusterMetadata defines metadata about a cluster.
type ClusterMetadata struct {
	NominatedStorageClass *StorageProvisioner
	OperatorStorageClass  *StorageProvisioner
	Cloud                 string
	Regions               set.Strings
	PreferredServiceType  string
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
