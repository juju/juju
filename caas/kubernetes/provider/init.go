// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"github.com/juju/juju/caas"
)

var k8sCloudCheckers map[string][]k8slabels.Selector
var jujuPreferredWorkloadStorage map[string][]caas.PreferredStorage
var jujuPreferredOperatorStorage map[string][]caas.PreferredStorage

func init() {
	caas.RegisterContainerProvider(CAASProviderType, providerInstance)

	// k8sCloudCheckers is a collection of k8s node selector requirement definitions
	// used for detecting cloud provider from node labels.
	k8sCloudCheckers = compileK8sCloudCheckers()

	// jujuPreferredWorkloadStorage defines the opinionated storage
	// that Juju requires to be available on supported clusters.
	jujuPreferredWorkloadStorage = map[string][]caas.PreferredStorage{
		caas.K8sCloudMicrok8s: {{
			Name:        "hostpath",
			Provisioner: "microk8s.io/hostpath",
		}},
		caas.K8sCloudGCE: {{
			Name:        "GCE Persistent Disk",
			Provisioner: "kubernetes.io/gce-pd",
		}},
		caas.K8sCloudAzure: {{
			Name:        "Azure Disk",
			Provisioner: "kubernetes.io/azure-disk",
		}},
		caas.K8sCloudEC2: {{
			Name:        "EBS Volume",
			Provisioner: "kubernetes.io/aws-ebs",
		}},
		caas.K8sCloudOpenStack: {
			{
				Name:        "Cinder Disk",
				Provisioner: "cinder.csi.openstack.org",
				CSI:         true,
			},
			{
				Name:        "Cinder Disk",
				Provisioner: "csi-cinderplugin",
				CSI:         true,
			},
			{
				Name:        "Cinder Disk",
				Provisioner: "kubernetes.io/cinder",
			},
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
		caas.K8sCloudOpenStack: {
			// CDK on Openstack.
			newLabelRequirements(
				requirementParams{"juju.io/cloud", selection.Equals, []string{"openstack"}},
			),
		},
		// format - cloudType: requirements.
	}
}
