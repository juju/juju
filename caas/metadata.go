// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"fmt"

	"github.com/juju/collections/set"
)

const (
	// Microk8s is the nme use dfor microk8s clouds.
	Microk8s = "microk8s"

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
	Cloud                 string
	Regions               set.Strings
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
