// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
)

type K8sMetadataSuite struct {
	BaseSuite
}

var _ = gc.Suite(&K8sMetadataSuite{})

func newNode(labels map[string]string) core.Node {
	n := core.Node{}
	n.SetLabels(labels)
	return n
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
	nodes           *core.NodeList
}

var hostRegionsTestCases = []hostRegionTestcase{
	{
		expectedRegions: set.NewStrings(),
		nodes:           newNodeList(map[string]string{}),
	},
	{
		expectedRegions: set.NewStrings(),
		nodes: newNodeList(map[string]string{
			"cloud.google.com/gke-nodepool": "",
		}),
	},
	{
		expectedRegions: set.NewStrings(),
		nodes: newNodeList(map[string]string{
			"cloud.google.com/gke-os-distribution": "",
		}),
	},
	{
		expectedCloud:   "gce",
		expectedRegions: set.NewStrings(""),
		nodes: newNodeList(map[string]string{
			"cloud.google.com/gke-nodepool":        "",
			"cloud.google.com/gke-os-distribution": "",
		}),
	},
	{
		expectedCloud:   "gce",
		expectedRegions: set.NewStrings(""),
		nodes: newNodeList(map[string]string{
			"juju.io/cloud": "gce",
		}),
	},
	{
		expectedCloud:   "ec2",
		expectedRegions: set.NewStrings(""),
		nodes: newNodeList(map[string]string{
			"juju.io/cloud": "ec2",
		}),
	},
	{
		expectedCloud:   "azure",
		expectedRegions: set.NewStrings(""),
		nodes: newNodeList(map[string]string{
			"juju.io/cloud": "azure",
		}),
	},
	{
		expectedCloud:   "azure",
		expectedRegions: set.NewStrings(""),
		nodes: newNodeList(map[string]string{
			"kubernetes.azure.com/cluster": "",
		}),
	},
	{
		expectedCloud:   "ec2",
		expectedRegions: set.NewStrings(""),
		nodes: newNodeList(map[string]string{
			"manufacturer": "amazon_ec2",
		}),
	},
	{
		expectedRegions: set.NewStrings(),
		nodes: newNodeList(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
		}),
	},
	{
		expectedRegions: set.NewStrings(),
		nodes: newNodeList(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
			"cloud.google.com/gke-nodepool":            "",
		}),
	},
	{
		expectedRegions: set.NewStrings(),
		nodes: newNodeList(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
			"cloud.google.com/gke-os-distribution":     "",
		}),
	},
	{
		expectedCloud:   "gce",
		expectedRegions: set.NewStrings("a-fancy-region"),
		nodes: newNodeList(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
			"cloud.google.com/gke-nodepool":            "",
			"cloud.google.com/gke-os-distribution":     "",
		}),
	},
	{
		expectedCloud:   "azure",
		expectedRegions: set.NewStrings("a-fancy-region"),
		nodes: newNodeList(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
			"kubernetes.azure.com/cluster":             "",
		}),
	},
	{
		expectedCloud:   "ec2",
		expectedRegions: set.NewStrings("a-fancy-region"),
		nodes: newNodeList(map[string]string{
			"failure-domain.beta.kubernetes.io/region": "a-fancy-region",
			"manufacturer": "amazon_ec2",
		}),
	},
}

func newNodeList(labels map[string]string) *core.NodeList {
	return &core.NodeList{Items: []core.Node{newNode(labels)}}
}

func (s *K8sMetadataSuite) TestListHostCloudRegions(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	for i, v := range hostRegionsTestCases {
		c.Logf("test %d", i)
		gomock.InOrder(
			s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).
				Return(v.nodes, nil),
			s.mockStorageClass.EXPECT().List(v1.ListOptions{}).
				Return(&storagev1.StorageClassList{}, nil),
		)
		metadata, err := s.broker.GetClusterMetadata("")
		c.Check(err, jc.ErrorIsNil)
		c.Check(metadata.Cloud, gc.Equals, v.expectedCloud)
		c.Check(metadata.Regions, jc.DeepEquals, v.expectedRegions)
	}
}

func (s *K8sMetadataSuite) TestNoDefaultStorageClasses(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).
			Return(&core.NodeList{}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{{
				Provisioner: "a-provisioner",
				Parameters:  map[string]string{"foo": "bar"},
			}}}, nil),
	)
	metadata, err := s.broker.GetClusterMetadata("")
	c.Check(err, jc.ErrorIsNil)
	c.Check(metadata.NominatedStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Provisioner: "a-provisioner",
		Parameters:  map[string]string{"foo": "bar"},
	})
}

func (s *K8sMetadataSuite) TestNoDefaultStorageClassesTooMany(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).
			Return(&core.NodeList{}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{{
				Provisioner: "a-provisioner",
				Parameters:  map[string]string{"foo": "bar"},
			}, {
				Provisioner: "b-provisioner",
				Parameters:  map[string]string{"foo": "bar"},
			}}}, nil),
	)
	metadata, err := s.broker.GetClusterMetadata("")
	c.Check(err, jc.ErrorIsNil)
	c.Check(metadata.NominatedStorageClass, gc.IsNil)
}

func (s *K8sMetadataSuite) TestPreferDefaultStorageClass(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).
			Return(&core.NodeList{}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{{
				ObjectMeta:  v1.ObjectMeta{Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}},
				Provisioner: "a-provisioner",
				Parameters:  map[string]string{"foo": "bar"},
			}, {
				Provisioner: "b-provisioner",
				Parameters:  map[string]string{"foo": "bar"},
			}}}, nil),
	)
	metadata, err := s.broker.GetClusterMetadata("")
	c.Check(err, jc.ErrorIsNil)
	c.Check(metadata.NominatedStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Provisioner: "a-provisioner",
		Parameters:  map[string]string{"foo": "bar"},
	})
}

func (s *K8sMetadataSuite) TestBetaDefaultStorageClass(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).
			Return(&core.NodeList{}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{{
				ObjectMeta:  v1.ObjectMeta{Annotations: map[string]string{"storageclass.beta.kubernetes.io/is-default-class": "true"}},
				Provisioner: "a-provisioner",
				Parameters:  map[string]string{"foo": "bar"},
			}, {
				Provisioner: "b-provisioner",
				Parameters:  map[string]string{"foo": "bar"},
			}}}, nil),
	)
	metadata, err := s.broker.GetClusterMetadata("")
	c.Check(err, jc.ErrorIsNil)
	c.Check(metadata.NominatedStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Provisioner: "a-provisioner",
		Parameters:  map[string]string{"foo": "bar"},
	})
}

func (s *K8sMetadataSuite) TestUserSpecifiedStorageClasses(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).
			Return(&core.NodeList{Items: []core.Node{{ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"manufacturer": "amazon_ec2"},
			}}}}, nil),
		s.mockStorageClass.EXPECT().Get("foo", v1.GetOptions{}).
			Return(&storagev1.StorageClass{
				ObjectMeta:  v1.ObjectMeta{Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}},
				Provisioner: "a-provisioner",
				Parameters:  map[string]string{"foo": "bar"},
			}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{{
				Provisioner: "kubernetes.io/aws-ebs",
			}}}, nil),
	)
	metadata, err := s.broker.GetClusterMetadata("foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(metadata.NominatedStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Provisioner: "a-provisioner",
		Parameters:  map[string]string{"foo": "bar"},
	})
	c.Check(metadata.OperatorStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Provisioner: "kubernetes.io/aws-ebs",
	})
}

func (s *K8sMetadataSuite) TestOperatorStorageClassNoDefault(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).
			Return(&core.NodeList{Items: []core.Node{{ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"manufacturer": "amazon_ec2"},
			}}}}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{{
				Provisioner: "kubernetes.io/aws-ebs",
			}, {
				Provisioner: "kubernetes.io/aws-ebs",
				Parameters:  map[string]string{"foo": "bar"},
			}}}, nil),
	)
	metadata, err := s.broker.GetClusterMetadata("")
	c.Check(err, jc.ErrorIsNil)
	// More than one match so need to be explicit for workload storage.
	c.Check(metadata.NominatedStorageClass, gc.IsNil)
	// Take the first match for operator storage.
	c.Check(metadata.OperatorStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Provisioner: "kubernetes.io/aws-ebs",
	})
}

func (s *K8sMetadataSuite) TestOperatorStorageClassPrefersDefault(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).
			Return(&core.NodeList{Items: []core.Node{{ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"manufacturer": "amazon_ec2"},
			}}}}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{{
				Provisioner: "kubernetes.io/aws-ebs",
			}, {
				ObjectMeta:  v1.ObjectMeta{Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}},
				Provisioner: "kubernetes.io/aws-ebs",
				Parameters:  map[string]string{"foo": "bar"},
			}}}, nil),
	)
	metadata, err := s.broker.GetClusterMetadata("")
	c.Check(err, jc.ErrorIsNil)
	c.Check(metadata.NominatedStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Provisioner: "kubernetes.io/aws-ebs",
		Parameters:  map[string]string{"foo": "bar"},
	})
	c.Check(metadata.OperatorStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Provisioner: "kubernetes.io/aws-ebs",
		Parameters:  map[string]string{"foo": "bar"},
	})
}

func (s *K8sMetadataSuite) TestAnnotatedWorkloadStorageClass(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).
			Return(&core.NodeList{Items: []core.Node{{ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"manufacturer": "amazon_ec2"},
			}}}}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{{
				ObjectMeta: v1.ObjectMeta{
					Name: "juju-preferred-workload-storage",
					Annotations: map[string]string{
						"juju.io/workload-storage": "true",
					},
				},
				Provisioner: "kubernetes.io/aws-ebs",
				Parameters:  map[string]string{"foo": "bar"},
			}}}, nil),
	)
	metadata, err := s.broker.GetClusterMetadata("")
	c.Check(err, jc.ErrorIsNil)
	c.Check(metadata.NominatedStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Name:        "juju-preferred-workload-storage",
		Provisioner: "kubernetes.io/aws-ebs",
		Parameters:  map[string]string{"foo": "bar"},
	})
	c.Check(metadata.OperatorStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Name:        "juju-preferred-workload-storage",
		Provisioner: "kubernetes.io/aws-ebs",
		Parameters:  map[string]string{"foo": "bar"},
	})
}

func (s *K8sMetadataSuite) TestAnnotatedWorkloadAndOperatorStorageClass(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).
			Return(&core.NodeList{Items: []core.Node{{ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"manufacturer": "amazon_ec2"},
			}}}}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "juju-preferred-workload-storage",
						Annotations: map[string]string{
							"juju.io/workload-storage": "true",
						},
					},
					Provisioner: "kubernetes.io/aws-ebs",
					Parameters:  map[string]string{"foo": "bar"},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "juju-preferred-operator-storage",
						Annotations: map[string]string{
							"juju.io/operator-storage": "true",
						},
					},
					Provisioner: "kubernetes.io/aws-ebs",
					Parameters:  map[string]string{"foo": "bar"},
				},
			}}, nil),
	)
	metadata, err := s.broker.GetClusterMetadata("")
	c.Check(err, jc.ErrorIsNil)
	c.Check(metadata.NominatedStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Name:        "juju-preferred-workload-storage",
		Provisioner: "kubernetes.io/aws-ebs",
		Parameters:  map[string]string{"foo": "bar"},
	})
	c.Check(metadata.OperatorStorageClass, jc.DeepEquals, &caas.StorageProvisioner{
		Name:        "juju-preferred-operator-storage",
		Provisioner: "kubernetes.io/aws-ebs",
		Parameters:  map[string]string{"foo": "bar"},
	})
}

func (s *K8sMetadataSuite) TestCheckDefaultWorkloadStorageUnknownCluster(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.CheckDefaultWorkloadStorage("foo", nil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *K8sMetadataSuite) TestCheckDefaultWorkloadStorageNonpreferred(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.CheckDefaultWorkloadStorage("microk8s", &caas.StorageProvisioner{Provisioner: "foo"})
	c.Assert(err, jc.Satisfies, caas.IsNonPreferredStorageError)
	npse, ok := errors.Cause(err).(*caas.NonPreferredStorageError)
	c.Assert(ok, jc.IsTrue)
	c.Assert(npse.Provisioner, gc.Equals, "microk8s.io/hostpath")
}

func (s *K8sMetadataSuite) TestLabelSetToRequirements(c *gc.C) {
	labels := map[string]string{
		"foo":  "bar",
		"foo1": "bar1",
	}
	var out []string
	for _, v := range provider.LabelSetToRequirements(labels) {
		out = append(out, v.String())
	}
	c.Assert(strings.Join(out, ","), gc.DeepEquals, `foo=bar,foo1=bar1`)
}

func (s *K8sMetadataSuite) TestMergeSelectors(c *gc.C) {
	selector1 := k8slabels.SelectorFromSet(map[string]string{"foo": "bar"})
	selector2 := k8slabels.SelectorFromSet(map[string]string{"foo1": "bar1"})
	c.Assert(provider.MergeSelectors(selector1, selector2), gc.DeepEquals,
		k8slabels.SelectorFromSet(map[string]string{
			"foo":  "bar",
			"foo1": "bar1",
		}),
	)
}
