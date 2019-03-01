// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"os"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"github.com/juju/juju/caas"
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

func getCloudProviderFromNodeMeta(node core.Node) string {
	for k, checker := range k8sCloudCheckers {
		if checker.Matches(k8slabels.Set(node.GetLabels())) {
			return k
		}
	}
	// TODO - add microk8s node label check when available
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	hostLabel, _ := node.Labels["kubernetes.io/hostname"]
	if node.Name == hostname && hostLabel == hostname {
		return "microk8s"
	}
	return ""
}

// GetClusterMetadata implements ClusterMetadataChecker.
func (k *kubernetesClient) GetClusterMetadata(storageClass string) (*caas.ClusterMetadata, error) {
	var result caas.ClusterMetadata
	var err error
	result.Regions, err = k.listHostCloudRegions()
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine cluster region")
	}

	if storageClass != "" {
		sc, err := k.StorageV1().StorageClasses().Get(storageClass, v1.GetOptions{IncludeUninitialized: true})
		if err != nil && !k8serrors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			result.NominatedStorageClass = &caas.StorageProvisioner{
				Provisioner: sc.Provisioner,
				Parameters:  sc.Parameters,
			}
		}
	} else {
		storageClasses, err := k.StorageV1().StorageClasses().List(v1.ListOptions{})
		if err != nil {
			return nil, errors.Annotate(err, "listing storage classes")
		}
		for _, sc := range storageClasses.Items {
			if v, ok := sc.Annotations["storageclass.kubernetes.io/is-default-class"]; ok && v != "false" {
				result.NominatedStorageClass = &caas.StorageProvisioner{
					Provisioner: sc.Provisioner,
					Parameters:  sc.Parameters,
				}
				break
			}
		}
	}
	return &result, nil
}

const regionLabelName = "failure-domain.beta.kubernetes.io/region"

// listHostCloudRegions lists all the cloud regions that this cluster has worker nodes/instances running in.
func (k *kubernetesClient) listHostCloudRegions() (set.Strings, error) {
	// we only check 5 worker nodes as of now just run in the one region and
	// we are just looking for a running worker to sniff its region.
	nodes, err := k.CoreV1().Nodes().List(v1.ListOptions{Limit: 5})
	if err != nil {
		return nil, errors.Annotate(err, "listing nodes")
	}
	result := set.NewStrings()
	for _, n := range nodes.Items {
		var cloudRegion, v string
		var ok bool
		if v = getCloudProviderFromNodeMeta(n); v == "" {
			continue
		}
		cloudRegion += v
		if v, ok = n.Labels[regionLabelName]; ok && v != "" {
			cloudRegion += "/" + v
		}
		result.Add(cloudRegion)
	}
	return result, nil
}

// CheckDefaultWorkloadStorage implements ClusterMetadataChecker.
func (k *kubernetesClient) CheckDefaultWorkloadStorage(cluster string, storageProvisioner *caas.StorageProvisioner) error {
	preferredStorage, ok := clusterPreferredWorkloadStorage[cluster]
	if !ok {
		return errors.NotFoundf("cluster %q", cluster)
	}
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
