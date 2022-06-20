// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"
	"errors"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/environs"
)

type K8sMetadataSuite struct {
	BaseSuite
}

var _ = gc.Suite(&K8sMetadataSuite{})

var (
	annotatedOperatorStorage = &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "operator-storage",
			Annotations: map[string]string{
				"juju.is/operator-storage": "true",
			},
		},
	}

	annotatedWorkloadStorage = &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "workload-storage",
			Annotations: map[string]string{
				"juju.is/workload-storage": "true",
			},
		},
	}

	azureNode = newNode(map[string]string{
		"failure-domain.beta.kubernetes.io/region": "wallyworld-region",
		"kubernetes.azure.com/cluster":             "true",
	})

	azureStorageClass = &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "mynode",
		},
		Provisioner: "kubernetes.io/azure-disk",
	}

	defaultStorage = &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "default",
			Annotations: map[string]string{
				"storageclass.kubernetes.io/is-default-class": "true",
			},
		},
	}

	ec2Node = newNode(map[string]string{
		"failure-domain.beta.kubernetes.io/region": "wallyworld-region",
		"manufacturer": "amazon_ec2",
	})

	ec2StorageClass = &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "mynode",
		},
		Provisioner: "kubernetes.io/aws-ebs",
	}

	gceNode = newNode(map[string]string{
		"failure-domain.beta.kubernetes.io/region": "wallyworld-region",
		"cloud.google.com/gke-nodepool":            "true",
		"cloud.google.com/gke-os-distribution":     "true",
	})

	gceStorageClass = &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "mynode",
		},
		Provisioner: "kubernetes.io/gce-pd",
	}

	microk8sNode = newNode(map[string]string{
		"microk8s.io/cluster":                      "true",
		"failure-domain.beta.kubernetes.io/region": "wallyworld-region",
	})

	microk8sStorageClass = &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "mynode",
		},
		Provisioner: "microk8s.io/hostpath",
	}

	nominatedStorage = &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "nominated",
		},
	}
)

func newNode(labels map[string]string) *core.Node {
	n := core.Node{}
	n.SetLabels(labels)
	return &n
}

func (s *K8sMetadataSuite) TestMicrok8sFromNodeMeta(c *gc.C) {
	node := core.Node{
		ObjectMeta: v1.ObjectMeta{
			Name:   "mynode",
			Labels: map[string]string{"microk8s.io/cluster": "true"},
		},
	}
	cloud, region := provider.GetCloudProviderFromNodeMeta(node)
	c.Assert(cloud, gc.Equals, "microk8s")
	c.Assert(region, gc.Equals, "localhost")
}

func (s *K8sMetadataSuite) TestMicrok8sWithRegionFromNodeMeta(c *gc.C) {
	node := core.Node{
		ObjectMeta: v1.ObjectMeta{
			Name: "mynode",
			Labels: map[string]string{
				"microk8s.io/cluster":                      "true",
				"failure-domain.beta.kubernetes.io/region": "somewhere",
			},
		},
	}
	cloud, region := provider.GetCloudProviderFromNodeMeta(node)
	c.Assert(cloud, gc.Equals, "microk8s")
	c.Assert(region, gc.Equals, "somewhere")
}

func (s *K8sMetadataSuite) TestK8sCloudCheckersValidationPass(c *gc.C) {
	// CompileK8sCloudCheckers will panic if there is invalid requirement definition so check it by calling it.
	cloudCheckers := provider.CompileK8sCloudCheckers()
	c.Assert(cloudCheckers, gc.NotNil)
}

type hostRegionTestcase struct {
	expectedCloud   string
	expectedRegions set.Strings
	node            *core.Node
}

var hostRegionsTestCases = []hostRegionTestcase{
	{
		expectedRegions: set.NewStrings(),
		node:            newNode(map[string]string{}),
	},
	{
		expectedRegions: set.NewStrings(),
		node: newNode(map[string]string{
			"cloud.google.com/gke-nodepool": "",
		}),
	},
	{
		expectedRegions: set.NewStrings(),
		node: newNode(map[string]string{
			"cloud.google.com/gke-os-distribution": "",
		}),
	},
	{
		expectedCloud:   "gce",
		expectedRegions: set.NewStrings(""),
		node: newNode(map[string]string{
			"cloud.google.com/gke-nodepool":        "",
			"cloud.google.com/gke-os-distribution": "",
		}),
	},
	{
		expectedCloud:   "gce",
		expectedRegions: set.NewStrings(""),
		node: newNode(map[string]string{
			"juju.is/cloud": "gce",
		}),
	},
	{
		expectedCloud:   "ec2",
		expectedRegions: set.NewStrings(""),
		node: newNode(map[string]string{
			"juju.is/cloud": "ec2",
		}),
	},
	{
		expectedCloud:   "azure",
		expectedRegions: set.NewStrings(""),
		node: newNode(map[string]string{
			"juju.is/cloud": "azure",
		}),
	},
	{
		expectedCloud:   "azure",
		expectedRegions: set.NewStrings(""),
		node: newNode(map[string]string{
			"kubernetes.azure.com/cluster": "",
		}),
	},
	{
		expectedCloud:   "ec2",
		expectedRegions: set.NewStrings(""),
		node: newNode(map[string]string{
			"manufacturer": "amazon_ec2",
		}),
	},
	{
		expectedCloud:   "ec2",
		expectedRegions: set.NewStrings(""),
		node: newNode(map[string]string{
			"eks.amazonaws.com/nodegroup": "any-node-group",
		}),
	},
	{
		expectedRegions: set.NewStrings(),
		node: newNode(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
		}),
	},
	{
		expectedRegions: set.NewStrings(),
		node: newNode(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
			"cloud.google.com/gke-nodepool":            "",
		}),
	},
	{
		expectedRegions: set.NewStrings(),
		node: newNode(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
			"cloud.google.com/gke-os-distribution":     "",
		}),
	},
	{
		expectedCloud:   "gce",
		expectedRegions: set.NewStrings("a-fancy-region"),
		node: newNode(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
			"cloud.google.com/gke-nodepool":            "",
			"cloud.google.com/gke-os-distribution":     "",
		}),
	},
	{
		expectedCloud:   "azure",
		expectedRegions: set.NewStrings("a-fancy-region"),
		node: newNode(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
			"kubernetes.azure.com/cluster":             "",
		}),
	},
	{
		expectedCloud:   "ec2",
		expectedRegions: set.NewStrings("a-fancy-region"),
		node: newNode(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
			"manufacturer": "amazon_ec2",
		}),
	},
}

func (s *K8sMetadataSuite) TestListHostCloudRegions(c *gc.C) {
	for _, v := range hostRegionsTestCases {
		clientSet := fake.NewSimpleClientset(v.node)

		metadata, err := provider.GetClusterMetadata(
			context.TODO(),
			"",
			clientSet.CoreV1().Nodes(),
			clientSet.StorageV1().StorageClasses(),
		)
		c.Check(err, jc.ErrorIsNil)
		c.Check(metadata.Cloud, gc.Equals, v.expectedCloud)
		c.Check(metadata.Regions, jc.DeepEquals, v.expectedRegions)
	}
}

func (_ *K8sMetadataSuite) TestGetMetadataVariations(c *gc.C) {
	tests := []struct {
		Name             string
		InitialObjects   []runtime.Object
		NominatedStorage string
		Result           kubernetes.ClusterMetadata
	}{
		// EC2 tests
		{
			Name: "Test ec2 cloud finds provisioner storage",
			InitialObjects: []runtime.Object{
				ec2Node,
				ec2StorageClass,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "ec2",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: ec2StorageClass,
				OperatorStorageClass: ec2StorageClass,
			},
		},
		{
			Name: "Test ec2 cloud prefers annotation storage",
			InitialObjects: []runtime.Object{
				ec2Node,
				ec2StorageClass,
				annotatedOperatorStorage,
				annotatedWorkloadStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "ec2",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: annotatedWorkloadStorage,
				OperatorStorageClass: annotatedOperatorStorage,
			},
		},
		{
			Name: "Test ec2 cloud prefers annotation storage without workload",
			InitialObjects: []runtime.Object{
				ec2Node,
				annotatedOperatorStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "ec2",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nil,
				OperatorStorageClass: annotatedOperatorStorage,
			},
		},
		{
			Name: "Test ec2 cloud prefers nominated storage as first priority",
			InitialObjects: []runtime.Object{
				ec2Node,
				ec2StorageClass,
				annotatedOperatorStorage,
				annotatedWorkloadStorage,
				nominatedStorage,
			},
			NominatedStorage: "nominated",
			Result: kubernetes.ClusterMetadata{
				Cloud:                "ec2",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nominatedStorage,
				OperatorStorageClass: nominatedStorage,
			},
		},
		{
			Name: "Test ec2 cloud with no found storage",
			InitialObjects: []runtime.Object{
				ec2Node,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "ec2",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nil,
				OperatorStorageClass: nil,
			},
		},
		{
			Name: "Test ec2 cloud with default storage",
			InitialObjects: []runtime.Object{
				ec2Node,
				defaultStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "ec2",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: defaultStorage,
				OperatorStorageClass: defaultStorage,
			},
		},

		// Microk8s
		{
			Name: "Test microk8s cloud finds provisioner storage",
			InitialObjects: []runtime.Object{
				microk8sNode,
				microk8sStorageClass,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "microk8s",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: microk8sStorageClass,
				OperatorStorageClass: microk8sStorageClass,
			},
		},
		{
			Name: "Test microk8s cloud prefers annotation storage",
			InitialObjects: []runtime.Object{
				microk8sNode,
				microk8sStorageClass,
				annotatedOperatorStorage,
				annotatedWorkloadStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "microk8s",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: annotatedWorkloadStorage,
				OperatorStorageClass: annotatedOperatorStorage,
			},
		},
		{
			Name: "Test microk8s cloud prefers annotation storage without workload",
			InitialObjects: []runtime.Object{
				microk8sNode,
				annotatedOperatorStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "microk8s",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nil,
				OperatorStorageClass: annotatedOperatorStorage,
			},
		},
		{
			Name: "Test microk8s cloud prefers nominated storage as first priority",
			InitialObjects: []runtime.Object{
				microk8sNode,
				microk8sStorageClass,
				annotatedOperatorStorage,
				annotatedWorkloadStorage,
				nominatedStorage,
			},
			NominatedStorage: "nominated",
			Result: kubernetes.ClusterMetadata{
				Cloud:                "microk8s",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nominatedStorage,
				OperatorStorageClass: nominatedStorage,
			},
		},
		{
			Name: "Test microk8s cloud with no found storage",
			InitialObjects: []runtime.Object{
				microk8sNode,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "microk8s",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nil,
				OperatorStorageClass: nil,
			},
		},
		{
			Name: "Test microk8s cloud doesn't use default storage",
			InitialObjects: []runtime.Object{
				microk8sNode,
				defaultStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "microk8s",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nil,
				OperatorStorageClass: nil,
			},
		},

		// Azure
		{
			Name: "Test azure cloud finds provisioner storage",
			InitialObjects: []runtime.Object{
				azureNode,
				azureStorageClass,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "azure",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: azureStorageClass,
				OperatorStorageClass: azureStorageClass,
			},
		},
		{
			Name: "Test azure cloud prefers annotation storage",
			InitialObjects: []runtime.Object{
				azureNode,
				azureStorageClass,
				annotatedOperatorStorage,
				annotatedWorkloadStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "azure",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: annotatedWorkloadStorage,
				OperatorStorageClass: annotatedOperatorStorage,
			},
		},
		{
			Name: "Test azure cloud prefers annotation storage without workload",
			InitialObjects: []runtime.Object{
				azureNode,
				annotatedOperatorStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "azure",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nil,
				OperatorStorageClass: annotatedOperatorStorage,
			},
		},
		{
			Name: "Test azure cloud prefers nominated storage as first priority",
			InitialObjects: []runtime.Object{
				azureNode,
				azureStorageClass,
				annotatedOperatorStorage,
				annotatedWorkloadStorage,
				nominatedStorage,
			},
			NominatedStorage: "nominated",
			Result: kubernetes.ClusterMetadata{
				Cloud:                "azure",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nominatedStorage,
				OperatorStorageClass: nominatedStorage,
			},
		},
		{
			Name: "Test azure cloud with no found storage",
			InitialObjects: []runtime.Object{
				azureNode,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "azure",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nil,
				OperatorStorageClass: nil,
			},
		},
		{
			Name: "Test azure cloud with default storage",
			InitialObjects: []runtime.Object{
				azureNode,
				defaultStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "azure",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: defaultStorage,
				OperatorStorageClass: defaultStorage,
			},
		},

		// GCE
		{
			Name: "Test gce cloud finds provisioner storage",
			InitialObjects: []runtime.Object{
				gceNode,
				gceStorageClass,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "gce",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: gceStorageClass,
				OperatorStorageClass: gceStorageClass,
			},
		},
		{
			Name: "Test gce cloud prefers annotation storage",
			InitialObjects: []runtime.Object{
				gceNode,
				gceStorageClass,
				annotatedOperatorStorage,
				annotatedWorkloadStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "gce",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: annotatedWorkloadStorage,
				OperatorStorageClass: annotatedOperatorStorage,
			},
		},
		{
			Name: "Test gce cloud prefers annotation storage without workload",
			InitialObjects: []runtime.Object{
				gceNode,
				annotatedOperatorStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "gce",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nil,
				OperatorStorageClass: annotatedOperatorStorage,
			},
		},
		{
			Name: "Test gce cloud prefers nominated storage as first priority",
			InitialObjects: []runtime.Object{
				gceNode,
				gceStorageClass,
				annotatedOperatorStorage,
				annotatedWorkloadStorage,
				nominatedStorage,
			},
			NominatedStorage: "nominated",
			Result: kubernetes.ClusterMetadata{
				Cloud:                "gce",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nominatedStorage,
				OperatorStorageClass: nominatedStorage,
			},
		},
		{
			Name: "Test gce cloud with no found storage",
			InitialObjects: []runtime.Object{
				gceNode,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "gce",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: nil,
				OperatorStorageClass: nil,
			},
		},
		{
			Name: "Test gce cloud with default storage",
			InitialObjects: []runtime.Object{
				gceNode,
				defaultStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "gce",
				Regions:              set.NewStrings("wallyworld-region"),
				WorkloadStorageClass: defaultStorage,
				OperatorStorageClass: defaultStorage,
			},
		},

		// Other
		{
			Name: "Test other cloud prefers annotation storage",
			InitialObjects: []runtime.Object{
				newNode(map[string]string{}),
				gceStorageClass,
				annotatedOperatorStorage,
				annotatedWorkloadStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "",
				Regions:              set.NewStrings(),
				WorkloadStorageClass: annotatedWorkloadStorage,
				OperatorStorageClass: annotatedOperatorStorage,
			},
		},
		{
			Name: "Test other cloud prefers annotation storage without workload",
			InitialObjects: []runtime.Object{
				newNode(map[string]string{}),
				annotatedOperatorStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "",
				Regions:              set.NewStrings(),
				WorkloadStorageClass: annotatedOperatorStorage,
				OperatorStorageClass: annotatedOperatorStorage,
			},
		},
		{
			Name: "Test other cloud prefers nominated storage as first priority",
			InitialObjects: []runtime.Object{
				newNode(map[string]string{}),
				gceStorageClass,
				annotatedOperatorStorage,
				annotatedWorkloadStorage,
				nominatedStorage,
			},
			NominatedStorage: "nominated",
			Result: kubernetes.ClusterMetadata{
				Cloud:                "",
				Regions:              set.NewStrings(),
				WorkloadStorageClass: nominatedStorage,
				OperatorStorageClass: nominatedStorage,
			},
		},
		{
			Name: "Test other cloud with no found storage",
			InitialObjects: []runtime.Object{
				newNode(map[string]string{}),
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "",
				Regions:              set.NewStrings(),
				WorkloadStorageClass: nil,
				OperatorStorageClass: nil,
			},
		},
		{
			Name: "Test other cloud with default storage",
			InitialObjects: []runtime.Object{
				newNode(map[string]string{}),
				defaultStorage,
			},
			Result: kubernetes.ClusterMetadata{
				Cloud:                "",
				Regions:              set.NewStrings(),
				WorkloadStorageClass: defaultStorage,
				OperatorStorageClass: defaultStorage,
			},
		},
	}

	for _, test := range tests {
		c.Logf("running test %s", test.Name)
		clientSet := fake.NewSimpleClientset(test.InitialObjects...)

		metadata, err := provider.GetClusterMetadata(
			context.TODO(),
			test.NominatedStorage,
			clientSet.CoreV1().Nodes(),
			clientSet.StorageV1().StorageClasses(),
		)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(*metadata, jc.DeepEquals, test.Result)
	}
}

func (s *K8sMetadataSuite) TestNominatedStorageNotFound(c *gc.C) {
	clientSet := fake.NewSimpleClientset(
		newNode(map[string]string{}),
		gceStorageClass,
		annotatedOperatorStorage,
		annotatedWorkloadStorage,
	)

	_, err := provider.GetClusterMetadata(
		context.TODO(),
		"my-nominated-storage",
		clientSet.CoreV1().Nodes(),
		clientSet.StorageV1().StorageClasses(),
	)

	var notFoundError *environs.NominatedStorageNotFound
	c.Assert(err, gc.NotNil)
	c.Assert(errors.As(err, &notFoundError), jc.IsTrue)
	c.Assert(notFoundError.StorageName, gc.Equals, "my-nominated-storage")
}
