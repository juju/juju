// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"github.com/juju/juju/caas"
	k8sannotations "github.com/juju/juju/core/annotations"
)

// newLabelRequirements creates a list of k8s node label requirements.
// This should be called inside package init function to panic earlier
// if there is a invalid requirement definition.
func newLabelRequirements(rs ...requirementParams) k8slabels.Selector {
	s := k8slabels.NewSelector()
	for _, r := range rs {
		l, err := k8slabels.NewRequirement(r.key, r.operator, r.strValues)
		if err != nil {
			// this panic only happens if the compiled code is wrong.
			panic(errors.Annotatef(err, "incorrect requirement config %v", r))
		}
		s = s.Add(*l)
	}
	return s
}

// requirementParams defines parameters used to create a k8s label requirement.
type requirementParams struct {
	key       string
	operator  selection.Operator
	strValues []string
}

const regionLabelName = "failure-domain.beta.kubernetes.io/region"

func getCloudRegionFromNodeMeta(node core.Node) (string, string) {
	for cloudType, checkers := range k8sCloudCheckers {
		for _, checker := range checkers {
			if checker.Matches(k8slabels.Set(node.GetLabels())) {
				region := node.Labels[regionLabelName]
				if region == "" && cloudType == caas.K8sCloudMicrok8s {
					region = caas.Microk8sRegion
				}
				return cloudType, region
			}
		}
	}
	return "", ""
}

func isDefaultStorageClass(sc storage.StorageClass) bool {
	return k8sannotations.New(sc.GetAnnotations()).HasAny(
		map[string]string{
			"storageclass.kubernetes.io/is-default-class": "true",
			// Older clusters still use the beta annotation.
			"storageclass.beta.kubernetes.io/is-default-class": "true",
		},
	)
}

// handle CDK separately.
const (
	CDKOperatorStorageClassAnnotationKey = "juju.io/operator-storage"
	CDKWorkloadStorageClassAnnotationKey = "juju.io/workload-storage"
)

func discoverCDKStoragePreferences(storageClasses []storage.StorageClass, metaData *caas.ClusterMetadata) bool {
	var found bool
	for _, sc := range storageClasses {
		scAnnotations := k8sannotations.New(sc.GetAnnotations())
		caasSC := caasStorageProvisioner(sc)
		found = scAnnotations.Has(CDKWorkloadStorageClassAnnotationKey, "true")
		if found {
			metaData.NominatedStorageClass = caasSC
			metaData.OperatorStorageClass = caasSC
		}
		if scAnnotations.Has(CDKOperatorStorageClassAnnotationKey, "true") {
			metaData.OperatorStorageClass = caasSC
		}
	}
	return found
}

func caasStorageProvisioner(sc storage.StorageClass) *caas.StorageProvisioner {
	caasSc := &caas.StorageProvisioner{
		Name:        sc.Name,
		Provisioner: sc.Provisioner,
		Parameters:  sc.Parameters,
	}
	if sc.ReclaimPolicy != nil {
		caasSc.ReclaimPolicy = string(*sc.ReclaimPolicy)
	}
	return caasSc
}

// GetClusterMetadata implements ClusterMetadataChecker.
func (k *kubernetesClient) GetClusterMetadata(storageClass string) (*caas.ClusterMetadata, error) {
	var result caas.ClusterMetadata
	var err error
	result.Cloud, result.Regions, err = k.listHostCloudRegions()
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine cluster region")
	}

	if storageClass != "" {
		sc, err := k.client().StorageV1().StorageClasses().Get(storageClass, v1.GetOptions{IncludeUninitialized: true})
		if err != nil && !k8serrors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err == nil && sc != nil {
			result.NominatedStorageClass = caasStorageProvisioner(*sc)
		}
	}

	// We may have the workload storage but still need to look for operator storage.
	storageClasses, err := k.client().StorageV1().StorageClasses().List(v1.ListOptions{})
	if err != nil {
		return nil, errors.Annotate(err, "listing storage classes")
	}

	if discoverCDKStoragePreferences(storageClasses.Items, &result) {
		// This is a CDK cluster with preferred storage class defined.
		return &result, nil
	}

	var possibleWorkloadStorage, possibleOperatorStorage []*caas.StorageProvisioner
	preferredOperatorStorage, hasPreferredOperatorStorage := jujuPreferredOperatorStorage[result.Cloud]
	for _, sc := range storageClasses.Items {
		caasSC := caasStorageProvisioner(sc)
		isDefaultSc := isDefaultStorageClass(sc)
		if result.OperatorStorageClass == nil && hasPreferredOperatorStorage {
			if err := storageClassMatches(preferredOperatorStorage, caasSC); err == nil {
				if isDefaultSc {
					// Prefer operator storage from the default storage class.
					result.OperatorStorageClass = caasSC
				} else {
					possibleOperatorStorage = append(possibleOperatorStorage, caasSC)
				}
			}
		}
		if result.NominatedStorageClass == nil {
			if isDefaultSc {
				// no nominated storage class specified, so use the default one;
				result.NominatedStorageClass = caasSC
				break
			}
			possibleWorkloadStorage = append(possibleWorkloadStorage, caasSC)
		}
	}

	if result.OperatorStorageClass == nil && len(possibleOperatorStorage) > 0 {
		result.OperatorStorageClass = possibleOperatorStorage[0]
	}
	// Even if no storage class was marked as default for the cluster, if there's only
	// one of them, use it for workload storage.
	if result.NominatedStorageClass == nil && len(possibleWorkloadStorage) > 0 {
		result.NominatedStorageClass = possibleWorkloadStorage[0]
	}
	if result.OperatorStorageClass == nil {
		// use workload storage class if no operator storage class preference found.
		result.OperatorStorageClass = result.NominatedStorageClass
	}
	return &result, nil
}

// listHostCloudRegions lists all the cloud regions that this cluster has worker nodes/instances running in.
func (k *kubernetesClient) listHostCloudRegions() (string, set.Strings, error) {
	// we only check 5 worker nodes as of now just run in the one region and
	// we are just looking for a running worker to sniff its region.
	nodes, err := k.client().CoreV1().Nodes().List(v1.ListOptions{Limit: 5})
	if err != nil {
		return "", nil, errors.Annotate(err, "listing nodes")
	}
	result := set.NewStrings()
	var cloudResult string
	for _, n := range nodes.Items {
		var nodeCloud, region string
		if nodeCloud, region = getCloudRegionFromNodeMeta(n); nodeCloud == "" {
			continue
		}
		cloudResult = nodeCloud
		result.Add(region)
	}
	return cloudResult, result, nil
}

// CheckDefaultWorkloadStorage implements ClusterMetadataChecker.
func (k *kubernetesClient) CheckDefaultWorkloadStorage(cloudType string, storageProvisioner *caas.StorageProvisioner) error {
	preferredStorage, ok := jujuPreferredWorkloadStorage[cloudType]
	if !ok {
		return errors.NotFoundf("preferred workload storage for cloudType %q", cloudType)
	}
	return storageClassMatches(preferredStorage, storageProvisioner)
}

func storageClassMatches(preferredStorage caas.PreferredStorage, storageProvisioner *caas.StorageProvisioner) error {
	if storageProvisioner == nil || preferredStorage.Provisioner != storageProvisioner.Provisioner {
		return &caas.NonPreferredStorageError{PreferredStorage: preferredStorage}
	}
	for k, v := range preferredStorage.Parameters {
		param, ok := storageProvisioner.Parameters[k]
		if !ok || param != v {
			return errors.Annotatef(
				&caas.NonPreferredStorageError{PreferredStorage: preferredStorage},
				"storage class %q requires parameter %s=%s", preferredStorage.Name, k, v)
		}
	}
	return nil
}
