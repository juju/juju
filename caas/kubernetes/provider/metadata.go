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
	csiv1alpha1 "k8s.io/csi-api/pkg/apis/csi/v1alpha1"

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

const (
	operatorStorageClassAnnotationKey = "juju.io/operator-storage"
	workloadStorageClassAnnotationKey = "juju.io/workload-storage"
)

func caasStorageProvisioner(sc storage.StorageClass) *caas.StorageProvisioner {
	caasSc := &caas.StorageProvisioner{
		Name:        sc.Name,
		Provisioner: sc.Provisioner,
		Parameters:  sc.Parameters,
		Default:     isDefaultStorageClass(sc),
	}
	if len(sc.Annotations) > 0 {
		caasSc.Annotations = k8sannotations.New(sc.GetAnnotations())
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

	// We may have the workload storage but still need to look for operator storage.
	storageClassesRes, err := k.client().StorageV1().StorageClasses().List(v1.ListOptions{})
	if err != nil {
		return nil, errors.Annotate(err, "listing storage classes")
	}

	var defaultStorageClass *caas.StorageProvisioner
	var storageProvisioners, preferredWorkloadStorage, preferredOperatorStorage []*caas.StorageProvisioner
	for _, sc := range storageClassesRes.Items {
		sp := caasStorageProvisioner(sc)
		storageProvisioners = append(storageProvisioners, sp)
		if sp.Default {
			if defaultStorageClass != nil {
				return nil, errors.New("default storage class is ambiguious")
			}
			defaultStorageClass = sp
		}
	}

	// Match named storage class.
	if storageClass != "" {
		for _, sp := range storageProvisioners {
			if sp.Name == storageClass {
				result.NominatedStorageClass = sp
				logger.Debugf("Use %q for nominated storage class", storageClass)
				break
			}
		}
		if result.NominatedStorageClass == nil {
			logger.Debugf("Specified storage class %q not found", storageClass)
		}
	}

	// Match explicit annotations.
	if result.NominatedStorageClass == nil {
		for _, sp := range storageProvisioners {
			if sp.Annotations.Has(workloadStorageClassAnnotationKey, "true") {
				logger.Debugf("Use %q with annotations %v for nominated storage class", sp.Name, sp.Annotations)
				result.NominatedStorageClass = sp
				break
			}
		}
	}
	if result.OperatorStorageClass == nil {
		for _, sp := range storageProvisioners {
			if sp.Annotations.Has(operatorStorageClassAnnotationKey, "true") {
				logger.Debugf("Use %q with annotations %v for nominated storage class", sp.Name, sp.Annotations)
				result.OperatorStorageClass = sp
				break
			}
		}
	}

	// Select storage class based on preference.
	if result.NominatedStorageClass == nil {
		scMatches, err := k.storageClassMatches(jujuPreferredWorkloadStorage[result.Cloud], storageProvisioners)
		if err != nil && !caas.IsNonPreferredStorageError(err) {
			return nil, errors.Trace(err)
		}
		for _, match := range scMatches {
			if match.StorageProvisioner.Default {
				result.NominatedStorageClass = match.StorageProvisioner
				logger.Debugf(
					"Use the default Storage class %q for workload storage class because it also matches Juju preferred config %v",
					match.StorageProvisioner.Name, match.PreferredStorage,
				)
				break
			}
		}
		if result.NominatedStorageClass == nil {
			for _, match := range scMatches {
				if err := match.Valid(); err == nil {
					preferredWorkloadStorage = append(preferredWorkloadStorage, match.StorageProvisioner)
				}
			}
		}
	}
	if result.OperatorStorageClass == nil {
		scMatches, err := k.storageClassMatches(jujuPreferredOperatorStorage[result.Cloud], storageProvisioners)
		if err != nil && !caas.IsNonPreferredStorageError(err) {
			return nil, errors.Trace(err)
		}
		for _, match := range scMatches {
			if match.StorageProvisioner.Default {
				result.OperatorStorageClass = match.StorageProvisioner
				logger.Debugf(
					"Use the default Storage class %q for operator storage class because it also matches Juju preferred config %v",
					match.StorageProvisioner.Name, match.PreferredStorage,
				)
				break
			}
		}
		if result.OperatorStorageClass == nil {
			for _, match := range scMatches {
				if err := match.Valid(); err == nil {
					preferredOperatorStorage = append(preferredOperatorStorage, match.StorageProvisioner)
				}
			}
		}
	}

	// Use default storage class even if it doesn't match preferences.
	if result.NominatedStorageClass == nil && defaultStorageClass != nil {
		result.NominatedStorageClass = defaultStorageClass
		logger.Debugf("Use the default Storage class %q for workload storage", result.NominatedStorageClass.Name)
	}

	// Use the most preferred storage classes for the operator.
	if result.OperatorStorageClass == nil && len(preferredOperatorStorage) > 0 {
		result.OperatorStorageClass = preferredOperatorStorage[0]
		logger.Debugf("Use %q for operator storage class", result.OperatorStorageClass.Name)
	}

	// Use the most preferred storage class for the workload.
	if result.NominatedStorageClass == nil && len(preferredWorkloadStorage) > 0 {
		result.NominatedStorageClass = preferredWorkloadStorage[0]
		logger.Debugf("Use %q for nominated storage class", result.NominatedStorageClass.Name)
	}

	// Even if no storage class was marked as default for the cluster, if there's only
	// one of them, use it for workload storage.
	if result.NominatedStorageClass == nil && len(storageProvisioners) == 1 {
		result.NominatedStorageClass = storageProvisioners[0]
		logger.Debugf("Use %q for nominated storage class", result.NominatedStorageClass.Name)
	}

	// Use workload storage class if no operator storage class preference found.
	if result.OperatorStorageClass == nil && result.NominatedStorageClass != nil {
		result.OperatorStorageClass = result.NominatedStorageClass
		logger.Debugf("Use nominated storage class %q for operator storage class", result.OperatorStorageClass.Name)
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

func (k *kubernetesClient) listCSIDrivers() (set.Strings, error) {
	// Check for CSI support.
	apiResources, err := k.csiClient().Discovery().ServerResourcesForGroupVersion(csiv1alpha1.SchemeGroupVersion.String())
	if k8serrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotate(err, "failed to detect CSI support")
	} else if len(apiResources.APIResources) == 0 {
		return nil, nil
	}
	drivers, err := k.csiClient().CsiV1alpha1().CSIDrivers().List(v1.ListOptions{})
	if err != nil {
		return nil, errors.Annotate(err, "failed to list CSI drivers")
	}
	driverNames := set.NewStrings()
	for _, driver := range drivers.Items {
		driverNames.Add(driver.Name)
	}
	return driverNames, nil
}

// CheckDefaultWorkloadStorage implements ClusterMetadataChecker.
func (k *kubernetesClient) CheckDefaultWorkloadStorage(cloudType string, storageProvisioner *caas.StorageProvisioner) error {
	preferredStorage, ok := jujuPreferredWorkloadStorage[cloudType]
	if !ok {
		return errors.NotFoundf("preferred storage for cloudType %q", cloudType)
	}
	matches, err := k.storageClassMatches(preferredStorage, []*caas.StorageProvisioner{storageProvisioner})
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return errors.NotFoundf("available preferred storage for cloudType %q", cloudType)
	}
	return matches[0].Valid()
}

func (k *kubernetesClient) storageClassMatches(preferredStorage []caas.PreferredStorage,
	storageProvisioners []*caas.StorageProvisioner) ([]storageClassMatchResult, error) {
	csiDrivers, err := k.listCSIDrivers()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var available []caas.PreferredStorage
	for _, ps := range preferredStorage {
		if ps.CSI && !csiDrivers.Contains(ps.Provisioner) {
			// CSI driver not found.
			continue
		}
		available = append(available, ps)
	}

	if len(available) == 0 {
		return nil, nil
	}

	var matches []storageClassMatchResult

	for _, ps := range available {
		for _, storageProvisioner := range storageProvisioners {
			if storageProvisioner.Provisioner == ps.Provisioner {
				matches = append(matches, storageClassMatchResult{ps, storageProvisioner})
			}
		}
	}

	if matches != nil {
		return matches, nil
	}

	return nil, &caas.NonPreferredStorageError{PreferredStorage: available[0]}
}

type storageClassMatchResult struct {
	PreferredStorage   caas.PreferredStorage
	StorageProvisioner *caas.StorageProvisioner
}

func (r *storageClassMatchResult) Valid() error {
	for k, v := range r.PreferredStorage.Parameters {
		param, ok := r.StorageProvisioner.Parameters[k]
		if !ok || param != v {
			return errors.Annotatef(
				&caas.NonPreferredStorageError{PreferredStorage: r.PreferredStorage},
				"storage class %q requires parameter %s=%s", r.PreferredStorage.Name, k, v)
		}
	}
	return nil
}
