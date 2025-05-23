// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

var (
	k8sCloudCheckers map[string][]k8slabels.Selector

	// LifecycleModelTeardownSelector is the label selector for removing global resources for model teardown.
	lifecycleModelTeardownSelector k8slabels.Selector
)

func init() {
	klog.SetLogger(newKlogAdaptor())

	caas.RegisterContainerProvider(constants.CAASProviderType, providerInstance)

	// k8sCloudCheckers is a collection of k8s node selector requirement definitions
	// used for detecting cloud provider from node labels.
	k8sCloudCheckers = compileK8sCloudCheckers()

	lifecycleModelTeardownSelector = compileLifecycleModelTeardownSelector()

}

// compileK8sCloudCheckers compiles/validates the collection of
// k8s node selector requirement definitions used for detecting
// cloud provider from node labels.
func compileK8sCloudCheckers() map[string][]k8slabels.Selector {
	return map[string][]k8slabels.Selector{
		k8s.K8sCloudMicrok8s: {
			newLabelRequirements(
				requirementParams{"microk8s.io/cluster", selection.Exists, nil},
			),
		},
		k8s.K8sCloudGCE: {
			// GKE.
			newLabelRequirements(
				requirementParams{"cloud.google.com/gke-nodepool", selection.Exists, nil},
				requirementParams{"cloud.google.com/gke-os-distribution", selection.Exists, nil},
			),
			// CDK on GCE.
			newLabelRequirements(
				requirementParams{"juju.is/cloud", selection.Equals, []string{"gce"}},
			),
		},
		k8s.K8sCloudEC2: {
			// EKS.
			newLabelRequirements(
				requirementParams{"manufacturer", selection.Equals, []string{"amazon_ec2"}},
			),
			newLabelRequirements(
				requirementParams{"eks.amazonaws.com/nodegroup", selection.Exists, nil},
			),
			// CDK on AWS.
			newLabelRequirements(
				requirementParams{"juju.is/cloud", selection.Equals, []string{"ec2"}},
			),
		},
		k8s.K8sCloudAzure: {
			// AKS.
			newLabelRequirements(
				requirementParams{"kubernetes.azure.com/cluster", selection.Exists, nil},
			),
			// CDK on Azure.
			newLabelRequirements(
				requirementParams{"juju.is/cloud", selection.Equals, []string{"azure"}},
			),
		},
		// format - cloudType: requirements.
	}
}

func compileLifecycleModelTeardownSelector() k8slabels.Selector {
	return newLabelRequirements(
		requirementParams{
			labelResourceLifeCycleKey, selection.NotIn, []string{
				labelResourceLifeCycleValuePersistent,
			}},
	)
}
