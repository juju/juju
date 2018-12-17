// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/caas"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

const (
	providerType = "kubernetes"
)

var k8sCloudCheckers map[string]k8slabels.Selector

func init() {
	caas.RegisterContainerProvider(providerType, providerInstance)

	k8sCloudCheckers = map[string]k8slabels.Selector{
		"gce": newLabelRequirements(
			requirement{"cloud.google.com/gke-nodepool", selection.Exists, nil},
			requirement{"cloud.google.com/gke-os-distribution", selection.Exists, nil},
		),
		"ec2": newLabelRequirements(
			requirement{"manufacturer", selection.Equals, []string{"amazon_ec2"}},
		),
		"azure": newLabelRequirements(
			requirement{"kubernetes.azure.com/cluster", selection.Exists, nil},
		),
		// format - cloudType: requirements.
		// TODO(ycliuhw): add support for cdk, etc.
	}
}
