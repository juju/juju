// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"github.com/juju/juju/caas"
)

const (
	providerType = "kubernetes"
)

var k8sCloudCheckers map[string]k8slabels.Selector

func init() {
	caas.RegisterContainerProvider(providerType, providerInstance)

	// k8sCloudCheckers is a collection of k8s node selector requirement definitions
	// used for detecting cloud provider from node labels.
	k8sCloudCheckers = compileK8sCloudCheckers()
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
