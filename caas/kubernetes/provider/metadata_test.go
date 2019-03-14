// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"os"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	hostname, err := os.Hostname()
	c.Assert(err, jc.ErrorIsNil)
	hostname = strings.ToLower(hostname)
	node := core.Node{
		ObjectMeta: v1.ObjectMeta{
			Name:   hostname,
			Labels: map[string]string{"kubernetes.io/hostname": hostname},
		},
	}
	cloud, region := provider.GetCloudProviderFromNodeMeta(node)
	c.Assert(cloud, gc.Equals, "microk8s")
	c.Assert(region, gc.Equals, "localhost")
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
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	for i, v := range hostRegionsTestCases {
		c.Logf("test %d", i)
		gomock.InOrder(
			s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).Times(1).
				Return(v.nodes, nil),
			s.mockStorageClass.EXPECT().List(v1.ListOptions{}).Times(1).
				Return(&storagev1.StorageClassList{}, nil),
		)
		metadata, err := s.broker.GetClusterMetadata("")
		c.Check(err, jc.ErrorIsNil)
		c.Check(metadata.Cloud, gc.Equals, v.expectedCloud)
		c.Check(metadata.Regions, jc.DeepEquals, v.expectedRegions)
	}
}

func (s *K8sMetadataSuite) TestNoDefaultStorageClasses(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).Times(1).
			Return(&core.NodeList{}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).Times(1).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{{}}}, nil),
	)
	metadata, err := s.broker.GetClusterMetadata("")
	c.Check(err, jc.ErrorIsNil)
	c.Check(metadata.NominatedStorageClass, gc.IsNil)
}

func (s *K8sMetadataSuite) TestDefaultStorageClasses(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).Times(1).
			Return(&core.NodeList{}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).Times(1).
			Return(&storagev1.StorageClassList{Items: []storagev1.StorageClass{{
				ObjectMeta:  v1.ObjectMeta{Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}},
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

func (s *K8sMetadataSuite) TestUserSpecifiedStorageClasses(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNodes.EXPECT().List(v1.ListOptions{Limit: 5}).Times(1).
			Return(&core.NodeList{Items: []core.Node{{ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"manufacturer": "amazon_ec2"},
			}}}}, nil),
		s.mockStorageClass.EXPECT().Get("foo", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&storagev1.StorageClass{
				ObjectMeta:  v1.ObjectMeta{Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}},
				Provisioner: "a-provisioner",
				Parameters:  map[string]string{"foo": "bar"},
			}, nil),
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).Times(1).
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

func (s *K8sMetadataSuite) TestCheckDefaultWorkloadStorageUnknownCluster(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	err := s.broker.CheckDefaultWorkloadStorage("foo", nil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *K8sMetadataSuite) TestCheckDefaultWorkloadStorageNonpreferred(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	err := s.broker.CheckDefaultWorkloadStorage("microk8s", &caas.StorageProvisioner{Provisioner: "foo"})
	c.Assert(err, jc.Satisfies, caas.IsNonPreferredStorageError)
	npse, ok := errors.Cause(err).(*caas.NonPreferredStorageError)
	c.Assert(ok, jc.IsTrue)
	c.Assert(npse.Provisioner, gc.Equals, "microk8s.io/hostpath")
}
