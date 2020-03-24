// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/paths"
)

const volBindModeWaitFirstConsumer = "WaitForFirstConsumer"

var (
	k8sCloudCheckers             map[string][]k8slabels.Selector
	jujuPreferredWorkloadStorage map[string]caas.PreferredStorage
	jujuPreferredOperatorStorage map[string]caas.PreferredStorage

	// lifecycleApplicationRemovalSelector is the label selector for removing global resources for application removal.
	lifecycleApplicationRemovalSelector k8slabels.Selector

	// LifecycleModelTeardownSelector is the label selector for removing global resources for model teardown.
	lifecycleModelTeardownSelector k8slabels.Selector

	k8sStorageBaseDir string
)

func init() {
	caas.RegisterContainerProvider(CAASProviderType, providerInstance)

	// k8sCloudCheckers is a collection of k8s node selector requirement definitions
	// used for detecting cloud provider from node labels.
	k8sCloudCheckers = compileK8sCloudCheckers()

	// jujuPreferredWorkloadStorage defines the opinionated storage
	// that Juju requires to be available on supported clusters.
	jujuPreferredWorkloadStorage = map[string]caas.PreferredStorage{
		// WaitForFirstConsumer mode which will delay the binding and provisioning of a PersistentVolume until a
		// Pod using the PersistentVolumeClaim is created.
		// https://kubernetes.io/docs/concepts/storage/storage-classes/#volume-binding-mode
		caas.K8sCloudMicrok8s: {
			Name:              "hostpath",
			Provisioner:       "microk8s.io/hostpath",
			VolumeBindingMode: volBindModeWaitFirstConsumer,
		},
		caas.K8sCloudGCE: {
			Name:              "GCE Persistent Disk",
			Provisioner:       "kubernetes.io/gce-pd",
			VolumeBindingMode: volBindModeWaitFirstConsumer,
		},
		caas.K8sCloudAzure: {
			Name:              "Azure Disk",
			Provisioner:       "kubernetes.io/azure-disk",
			VolumeBindingMode: volBindModeWaitFirstConsumer,
		},
		caas.K8sCloudEC2: {
			Name:              "EBS Volume",
			Provisioner:       "kubernetes.io/aws-ebs",
			VolumeBindingMode: volBindModeWaitFirstConsumer,
		},
		caas.K8sCloudOpenStack: {
			Name:              "Cinder Disk",
			Provisioner:       "csi-cinderplugin",
			VolumeBindingMode: volBindModeWaitFirstConsumer,
		},
	}

	// jujuPreferredOperatorStorage defines the opinionated storage
	// that Juju requires to be available on supported clusters to
	// provision storage for operators.
	// TODO - support regional storage for GCE etc
	jujuPreferredOperatorStorage = jujuPreferredWorkloadStorage

	lifecycleApplicationRemovalSelector = compileLifecycleApplicationRemovalSelector()
	lifecycleModelTeardownSelector = compileLifecycleModelTeardownSelector()

	k8sStorageBaseDir = getK8sStorageBaseDir()
}

// compileK8sCloudCheckers compiles/validates the collection of
// k8s node selector requirement definitions used for detecting
// cloud provider from node labels.
func compileK8sCloudCheckers() map[string][]k8slabels.Selector {
	return map[string][]k8slabels.Selector{
		caas.K8sCloudMicrok8s: {
			newLabelRequirements(
				requirementParams{"microk8s.io/cluster", selection.Exists, nil},
			),
		},
		caas.K8sCloudGCE: {
			// GKE.
			newLabelRequirements(
				requirementParams{"cloud.google.com/gke-nodepool", selection.Exists, nil},
				requirementParams{"cloud.google.com/gke-os-distribution", selection.Exists, nil},
			),
			// CDK on GCE.
			newLabelRequirements(
				requirementParams{"juju.io/cloud", selection.Equals, []string{"gce"}},
			),
		},
		caas.K8sCloudEC2: {
			// EKS.
			newLabelRequirements(
				requirementParams{"manufacturer", selection.Equals, []string{"amazon_ec2"}},
			),
			// CDK on AWS.
			newLabelRequirements(
				requirementParams{"juju.io/cloud", selection.Equals, []string{"ec2"}},
			),
		},
		caas.K8sCloudAzure: {
			// AKS.
			newLabelRequirements(
				requirementParams{"kubernetes.azure.com/cluster", selection.Exists, nil},
			),
			// CDK on Azure.
			newLabelRequirements(
				requirementParams{"juju.io/cloud", selection.Equals, []string{"azure"}},
			),
		},
		// format - cloudType: requirements.
	}
}

func compileLifecycleApplicationRemovalSelector() k8slabels.Selector {
	return newLabelRequirements(
		requirementParams{
			labelResourceLifeCycleKey, selection.NotIn, []string{
				labelResourceLifeCycleValueModel,
				labelResourceLifeCycleValuePersistent,
			}},
	)
}

func compileLifecycleModelTeardownSelector() k8slabels.Selector {
	return newLabelRequirements(
		requirementParams{
			labelResourceLifeCycleKey, selection.NotIn, []string{
				labelResourceLifeCycleValuePersistent,
			}},
	)
}

func getK8sStorageBaseDir() string {
	s, err := paths.StorageDir(CAASProviderType)
	if err != nil {
		panic(err)
	}
	return s
}
