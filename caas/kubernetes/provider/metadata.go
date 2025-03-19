// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	core "k8s.io/client-go/kubernetes/typed/core/v1"
	storage "k8s.io/client-go/kubernetes/typed/storage/v1"

	"github.com/juju/juju/caas/kubernetes"
	providerstorage "github.com/juju/juju/caas/kubernetes/provider/storage"
	"github.com/juju/juju/environs"
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
			logger.Warningf(context.TODO(), "%v is not selectable", v)
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

func getCloudRegionFromNodeMeta(node corev1.Node) (string, string) {
	for cloudType, checkers := range k8sCloudCheckers {
		for _, checker := range checkers {
			if checker.Matches(k8slabels.Set(node.GetLabels())) {
				region := node.Labels[regionLabelName]
				if region == "" && cloudType == kubernetes.K8sCloudMicrok8s {
					region = kubernetes.Microk8sRegion
				}
				return cloudType, region
			}
		}
	}
	return "", ""
}

func toCaaSStorageProvisioner(sc *storagev1.StorageClass) *kubernetes.StorageProvisioner {
	caasSc := &kubernetes.StorageProvisioner{
		Name:        sc.Name,
		Provisioner: sc.Provisioner,
		Parameters:  sc.Parameters,
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
func (k *kubernetesClient) ValidateCloudEndpoint(ctx context.Context) error {
	_, err := k.GetClusterMetadata(ctx, "")
	return errors.Trace(err)
}

// GetClusterMetadata implements ClusterMetadataChecker. If a nominated storage
// class is provided
func (k *kubernetesClient) GetClusterMetadata(ctx context.Context, nominatedStorageClass string) (*kubernetes.ClusterMetadata, error) {
	return GetClusterMetadata(
		ctx,
		nominatedStorageClass,
		k.client().CoreV1().Nodes(),
		k.client().StorageV1().StorageClasses(),
	)
}

// GetClusterMetadata is responsible for gather a Kubernetes cluster metadata
// for Juju to make decisions. This relates to the cloud the cluster may or may
// not be running in + storage available. Split out from the main
// kubernetesClient struct so that it can be tested correctly.
func GetClusterMetadata(
	ctx context.Context,
	nominatedStorageClass string,
	nodeI core.NodeInterface,
	storageClassI storage.StorageClassInterface,
) (*kubernetes.ClusterMetadata, error) {
	var result kubernetes.ClusterMetadata
	var err error
	result.Cloud, result.Regions, err = listHostCloudRegions(ctx, nodeI)
	if err != nil {
		return nil, errors.Annotate(err, "cannot determine cluster region")
	}

	storageClasses, err := storageClassI.List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, errors.Annotate(err, "listing storage classes")
	}

	preferredWorkloadStorage := providerstorage.PreferredWorkloadStorageForCloud(result.Cloud).Prepend(
		&providerstorage.PreferredStorageNominated{
			StorageClassName: nominatedStorageClass,
		},
	)

	var (
		selectedWorkloadSC *storagev1.StorageClass
		workloadPriority   int
	)
	for i, sc := range storageClasses.Items {
		priority, matches := preferredWorkloadStorage.Matches(&sc)
		if matches && (priority < workloadPriority || selectedWorkloadSC == nil) {
			selectedWorkloadSC = &storageClasses.Items[i]
			workloadPriority = priority
		}
	}

	if nominatedStorageClass != "" {
		if selectedWorkloadSC == nil || selectedWorkloadSC.Name != nominatedStorageClass {
			return nil, &environs.NominatedStorageNotFound{
				StorageName: nominatedStorageClass,
			}
		}
	}

	result.WorkloadStorageClass = selectedWorkloadSC
	return &result, nil
}

// listHostCloudRegions lists all the cloud regions that this cluster has worker nodes/instances running in.
func listHostCloudRegions(
	ctx context.Context,
	nodeI core.NodeInterface,
) (string, set.Strings, error) {
	// we only check 5 worker nodes as of now just run in the one region and
	// we are just looking for a running worker to sniff its region.
	nodes, err := nodeI.List(ctx, v1.ListOptions{Limit: 5})
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
