// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	storage "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	k8s "github.com/juju/juju/caas/kubernetes"
	k8sannotations "github.com/juju/juju/core/annotations"
	environscontext "github.com/juju/juju/environs/context"
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

func labelSetToRequirements(labels k8slabels.Set) []k8slabels.Requirement {
	out, _ := k8slabels.SelectorFromValidatedSet(labels).Requirements()
	return out
}

func mergeSelectors(selectors ...k8slabels.Selector) k8slabels.Selector {
	s := k8slabels.NewSelector()
	for _, v := range selectors {
		if v.Empty() {
			continue
		}
		rs, selectable := v.Requirements()
		if selectable {
			s = s.Add(rs...)
		} else {
			logger.Warningf("%v is not selectable", v)
		}
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
				if region == "" && cloudType == k8s.K8sCloudMicrok8s {
					region = k8s.Microk8sRegion
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

const (
	operatorStorageClassAnnotationKey = "juju.is/operator-storage"
	workloadStorageClassAnnotationKey = "juju.is/workload-storage"
)

func toCaaSStorageProvisioner(sc storage.StorageClass) *k8s.StorageProvisioner {
	caasSc := &k8s.StorageProvisioner{
		Name:        sc.Name,
		Provisioner: sc.Provisioner,
		Parameters:  sc.Parameters,
		IsDefault:   isDefaultStorageClass(sc),
	}
	if sc.VolumeBindingMode != nil {
		caasSc.VolumeBindingMode = string(*sc.VolumeBindingMode)
	}
	if sc.ReclaimPolicy != nil {
		caasSc.ReclaimPolicy = string(*sc.ReclaimPolicy)
	}
	return caasSc
}

// ValidateCloudEndpoint returns nil if the current model can talk to the kubernetes
// endpoint.  Used as validation during model upgrades.
// Implements environs.CloudEndpointChecker
func (k *kubernetesClient) ValidateCloudEndpoint(_ environscontext.ProviderCallContext) error {
	_, err := k.GetClusterMetadata("")
	return errors.Trace(err)
}

// GetClusterMetadata implements ClusterMetadataChecker.
func (k *kubernetesClient) GetClusterMetadata(storageClass string) (*k8s.ClusterMetadata, error) {
	var result k8s.ClusterMetadata
	var err error
	result.Cloud, result.Regions, err = k.listHostCloudRegions()
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine cluster region")
	}

	if storageClass != "" {
		sc, err := k.client().StorageV1().StorageClasses().Get(context.TODO(), storageClass, v1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			logger.Debugf("use %q for nominated storage class", sc.Name)
			result.NominatedStorageClass = toCaaSStorageProvisioner(*sc)
		}
	}

	// We may have the workload storage but still need to look for operator storage.
	storageClasses, err := k.client().StorageV1().StorageClasses().List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return nil, errors.Annotate(err, "listing storage classes")
	}

	var possibleWorkloadStorage, possibleOperatorStorage []*k8s.StorageProvisioner
	preferredOperatorStorage, hasPreferredOperatorStorage := jujuPreferredOperatorStorage[result.Cloud]

	pickOperatorSC := func(sc storage.StorageClass, maybeStorage *k8s.StorageProvisioner) {
		if result.OperatorStorageClass != nil {
			return
		}

		if k8sannotations.New(sc.GetAnnotations()).Has(operatorStorageClassAnnotationKey, "true") {
			logger.Debugf("use %q with annotations %v for operator storage class", sc.Name, sc.GetAnnotations())
			result.OperatorStorageClass = maybeStorage
		} else if hasPreferredOperatorStorage {
			err := storageClassMatches(preferredOperatorStorage, maybeStorage)
			if err != nil {
				// not match.
				return
			}
			if isDefaultStorageClass(sc) {
				// Prefer operator storage from the default storage class.
				result.OperatorStorageClass = maybeStorage
				logger.Debugf(
					"use the default Storage class %q for operator storage class because it also matches Juju preferred config %v",
					maybeStorage.Name, preferredOperatorStorage,
				)
			} else {
				possibleOperatorStorage = append(possibleOperatorStorage, maybeStorage)
			}
		}
	}

	pickWorkloadSC := func(sc storage.StorageClass, maybeStorage *k8s.StorageProvisioner) {
		if result.NominatedStorageClass != nil {
			return
		}

		if k8sannotations.New(sc.GetAnnotations()).Has(workloadStorageClassAnnotationKey, "true") {
			logger.Debugf("use %q with annotations %v for nominated storage class", sc.Name, sc.GetAnnotations())
			result.NominatedStorageClass = maybeStorage
		} else if isDefaultStorageClass(sc) {
			// no nominated storage class specified, so use the default one;
			result.NominatedStorageClass = maybeStorage
			logger.Debugf("use the default Storage class %q for nominated storage class", maybeStorage.Name)
		} else {
			possibleWorkloadStorage = append(possibleWorkloadStorage, maybeStorage)
		}
	}

	for _, sc := range storageClasses.Items {
		if result.OperatorStorageClass != nil && result.NominatedStorageClass != nil {
			break
		}
		maybeStorage := toCaaSStorageProvisioner(sc)
		pickOperatorSC(sc, maybeStorage)
		pickWorkloadSC(sc, maybeStorage)
	}

	if result.OperatorStorageClass == nil && len(possibleOperatorStorage) > 0 {
		result.OperatorStorageClass = possibleOperatorStorage[0]
		logger.Debugf("use %q for operator storage class", possibleOperatorStorage[0].Name)
	}
	// Even if no storage class was marked as default for the cluster, if there's only
	// one of them, use it for workload storage.
	if result.NominatedStorageClass == nil && len(possibleWorkloadStorage) == 1 {
		result.NominatedStorageClass = possibleWorkloadStorage[0]
		logger.Debugf("use %q for nominated storage class", possibleWorkloadStorage[0].Name)
	}
	if result.OperatorStorageClass == nil && result.NominatedStorageClass != nil {
		// use workload storage class if no operator storage class preference found.
		result.OperatorStorageClass = result.NominatedStorageClass
		logger.Debugf("use nominated storage class %q for operator storage class", result.NominatedStorageClass.Name)
	}
	return &result, nil
}

// listHostCloudRegions lists all the cloud regions that this cluster has worker nodes/instances running in.
func (k *kubernetesClient) listHostCloudRegions() (string, set.Strings, error) {
	// we only check 5 worker nodes as of now just run in the one region and
	// we are just looking for a running worker to sniff its region.
	nodes, err := k.client().CoreV1().Nodes().List(context.TODO(), v1.ListOptions{Limit: 5})
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
func (k *kubernetesClient) CheckDefaultWorkloadStorage(cloudType string, storageProvisioner *k8s.StorageProvisioner) error {
	preferredStorage, ok := jujuPreferredWorkloadStorage[cloudType]
	if !ok {
		return errors.NotFoundf("preferred workload storage for cloudType %q", cloudType)
	}
	return storageClassMatches(preferredStorage, storageProvisioner)
}

func storageClassMatches(preferredStorage k8s.PreferredStorage, storageProvisioner *k8s.StorageProvisioner) error {
	if preferredStorage.SupportsDefault && storageProvisioner.IsDefault {
		return nil
	}

	if storageProvisioner == nil || preferredStorage.Provisioner != storageProvisioner.Provisioner {
		return &k8s.NonPreferredStorageError{PreferredStorage: preferredStorage}
	}
	for k, v := range preferredStorage.Parameters {
		param, ok := storageProvisioner.Parameters[k]
		if !ok || param != v {
			return errors.Annotatef(
				&k8s.NonPreferredStorageError{PreferredStorage: preferredStorage},
				"storage class %q requires parameter %s=%s", preferredStorage.Name, k, v)
		}
	}
	return nil
}
