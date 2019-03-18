// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"github.com/juju/juju/caas"
)

var k8sCloudCheckers map[string]k8slabels.Selector
var jujuPreferredWorkloadStorage map[string]caas.PreferredStorage
var jujuPreferredOperatorStorage map[string]caas.PreferredStorage

func init() {
	caas.RegisterContainerProvider(CAASProviderType, providerInstance)

	// k8sCloudCheckers is a collection of k8s node selector requirement definitions
	// used for detecting cloud provider from node labels.
	k8sCloudCheckers = compileK8sCloudCheckers()

	// jujuPreferredWorkloadStorage defines the opinionated storage
	// that Juju requires to be available on supported clusters.
	jujuPreferredWorkloadStorage = map[string]caas.PreferredStorage{
		"microk8s": {
			Name:        "hostpath",
			Provisioner: "microk8s.io/hostpath",
		},
		"gce": {
			Name:        "GCE Persistent Disk",
			Provisioner: "kubernetes.io/gce-pd",
		},
		"azure": {
			Name:        "Azure Disk",
			Provisioner: "kubernetes.io/azure-disk",
		},
		"ec2": {
			Name:        "EBS Volume",
			Provisioner: "kubernetes.io/aws-ebs",
		},
	}

	// jujuPreferredOperatorStorage defines the opinionated storage
	// that Juju requires to be available on supported clusters to
	// provision storage for operators.
	// TODO - support regional storage for GCE etc
	jujuPreferredOperatorStorage = jujuPreferredWorkloadStorage
}

// compileK8sCloudCheckers compiles/validates the collection of
// k8s node selector requirement definitions used for detecting
// cloud provider from node labels.
func compileK8sCloudCheckers() map[string]k8slabels.Selector {
	return map[string]k8slabels.Selector{
		"gce": newLabelRequirements(
			requirementParams{"cloud.google.com/gke-nodepool", selection.Exists, nil},
			requirementParams{"cloud.google.com/gke-os-distribution", selection.Exists, nil},
		),
		"ec2": newLabelRequirements(
			requirementParams{"manufacturer", selection.Equals, []string{"amazon_ec2"}},
		),
		"azure": newLabelRequirements(
			requirementParams{"kubernetes.azure.com/cluster", selection.Exists, nil},
		),
		// format - cloudType: requirements.
		// TODO(caas): add support for cdk, etc.
	}
}
