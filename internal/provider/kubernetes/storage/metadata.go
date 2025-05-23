// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	storagev1 "k8s.io/api/storage/v1"

	"github.com/juju/juju/caas/kubernetes"
	k8sannotations "github.com/juju/juju/core/annotations"
)

// PreferredStorageAny is an implementation of PreferredStorage that matches
// any storage class which isn't nil
type PreferredStorageAny struct{}

// PreferredStorageNominated is an implementation of PreferredStorage that
// matches based on a nominated storage class name.
type PreferredStorageNominated struct {
	// StorageClassName is the name to match to for a Kubernetes storage class.
	StorageClassName string
}

// PreferredStorage defines a matcher that can inject it's own logic into the
// preferred storage choice for a given storage class and decide if it's a match
// to it's rules.
type PreferredStorage interface {
	// Name returns the name used to uniquely identify this preferred storage
	// matcher.
	Name() string

	// Matches returns true if this matcher matches the supplied storage class.
	Matches(*storagev1.StorageClass) bool
}

// PreferredStorageWorkloadAnnotation is an implementation of PreferredStorage
// that matches based on the workload storage annotation.
type PreferredStorageWorkloadAnnotation struct{}

// PreferredStorageDefault is an implementation of PreferredStorage that returns
// true if the supplied Kubernetes storage class is considered the default
// storage class for the cluster.
type PreferredStorageDefault struct{}

// PreferredStorageProvisioner is an implementation of PreferredStorage that
// returns true if the supplied Kubernetes storage classes provisioner matches
// this provisioner.
type PreferredStorageProvisioner struct {
	// NameVal defines the value return by this struct's Name() method
	NameVal string

	// Provisioner is the string to match on the storage classess provisioner
	// member.
	Provisioner string
}

// PreferredStorageList defined an ordered list of PreferredStorage matches to
// test a given storage class against. The position of PreferredStorage matches
// matches indicating they're preference.
type PreferredStorageList []PreferredStorage

const (
	workloadStorageClassAnnotationKey = "juju.is/workload-storage"
)

// PreferredWorkloadStorageForCloud returns a PreferredStorageList for the
// supplied cloud. If no cloud is found matching then a default list is
// provided.
func PreferredWorkloadStorageForCloud(cloud string) PreferredStorageList {
	switch cloud {

	// Microk8s
	case kubernetes.K8sCloudMicrok8s:
		return PreferredStorageList{
			&PreferredStorageWorkloadAnnotation{},
			&PreferredStorageProvisioner{
				NameVal:     "hostpath",
				Provisioner: "microk8s.io/hostpath",
			},
		}

	// Azure
	case kubernetes.K8sCloudAzure:
		return PreferredStorageList{
			&PreferredStorageWorkloadAnnotation{},
			&PreferredStorageProvisioner{
				NameVal:     "azure-disk",
				Provisioner: "kubernetes.io/azure-disk",
			},
			&PreferredStorageDefault{},
		}

	// Google Cloud
	case kubernetes.K8sCloudGCE:
		return PreferredStorageList{
			&PreferredStorageWorkloadAnnotation{},
			&PreferredStorageProvisioner{
				NameVal:     "gce-pd",
				Provisioner: "kubernetes.io/gce-pd",
			},
			&PreferredStorageDefault{},
		}

	// AWS
	case kubernetes.K8sCloudEC2:
		return PreferredStorageList{
			&PreferredStorageWorkloadAnnotation{},
			&PreferredStorageProvisioner{
				NameVal:     "aws-ebs",
				Provisioner: "kubernetes.io/aws-ebs",
			},
			&PreferredStorageDefault{},
		}

	// Openstack
	case kubernetes.K8sCloudOpenStack:
		return PreferredStorageList{
			&PreferredStorageWorkloadAnnotation{},
			&PreferredStorageProvisioner{
				NameVal:     "csi-cinder",
				Provisioner: "csi-cinderplugin",
			},
			&PreferredStorageDefault{},
		}

	// Everything else
	default:
		return PreferredStorageList{
			&PreferredStorageWorkloadAnnotation{},
			&PreferredStorageDefault{},
			&PreferredStorageAny{},
		}
	}
}

// Prepend adds the supplied PreferredStorage matcher to the beginning of this
// list. This makes the PreferredStorage the highest storage matcher and the
// most preferred. Useful for when a user has nominated their own storage.
func (p PreferredStorageList) Prepend(ps PreferredStorage) PreferredStorageList {
	return append(PreferredStorageList{ps}, p...)
}

// Matches implements PreferredStorage Matches.
func (_ *PreferredStorageAny) Matches(sc *storagev1.StorageClass) bool {
	if sc == nil {
		return false
	}
	return true
}

// Matches is responsible for taking a Kubernetes StorageClass and testing it
// against each PreferredStorage matcher in this slice. The first match this
// function returns the preference of this storage class. Lower prefferences
// are better. If no match is return -1 and false is returned.
func (p PreferredStorageList) Matches(sc *storagev1.StorageClass) (int, bool) {
	for i, val := range []PreferredStorage(p) {
		if val.Matches(sc) {
			return i, true
		}
	}
	return -1, false
}

// Matches implements PreferredStorage Matches.
func (p *PreferredStorageNominated) Matches(sc *storagev1.StorageClass) bool {
	if p.StorageClassName == "" || sc == nil {
		return false
	}
	return sc.Name == p.StorageClassName
}

// Matches implements PreferredStorage Matches.
func (_ *PreferredStorageWorkloadAnnotation) Matches(sc *storagev1.StorageClass) bool {
	return k8sannotations.New(sc.GetAnnotations()).Has(
		workloadStorageClassAnnotationKey, "true",
	)
}

// Matches implements PreferredStorage Matches.
func (p *PreferredStorageDefault) Matches(sc *storagev1.StorageClass) bool {
	return k8sannotations.New(sc.GetAnnotations()).HasAny(
		map[string]string{
			"storageclass.kubernetes.io/is-default-class": "true",
			// Older clusters still use the beta annotation.
			"storageclass.beta.kubernetes.io/is-default-class": "true",
		},
	)
}

// Matches implements PreferredStorage Matches.
func (p *PreferredStorageProvisioner) Matches(sc *storagev1.StorageClass) bool {
	return sc.Provisioner == p.Provisioner
}

// Name implements PreferredStorage Name.
func (_ *PreferredStorageAny) Name() string {
	return "any"
}

// Name implements PreferredStorage Name.
func (_ *PreferredStorageNominated) Name() string {
	return "nominated-storage"
}

// Name implements PreferredStorage Name.
func (_ *PreferredStorageWorkloadAnnotation) Name() string {
	return "workload-storage-annotation"
}

// Name implements PreferredStorage Name.
func (p *PreferredStorageDefault) Name() string {
	return "cluster-default-storage"
}

// Name implements PreferredStorage Name.
func (p *PreferredStorageProvisioner) Name() string {
	return p.NameVal
}
