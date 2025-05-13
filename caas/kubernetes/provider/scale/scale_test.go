// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scale_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas/kubernetes/provider/scale"
)

type ScaleSuite struct {
	client *fake.Clientset
}

var _ = tc.Suite(&ScaleSuite{})

func (s *ScaleSuite) SetUpTest(c *tc.C) {
	s.client = fake.NewSimpleClientset()
	_, err := s.client.CoreV1().Namespaces().Create(
		context.Background(),
		&core.Namespace{
			ObjectMeta: meta.ObjectMeta{
				Name: "test",
			},
		},
		meta.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ScaleSuite) TestDeploymentScale(c *tc.C) {
	_, err := s.client.AppsV1().Deployments("test").Create(
		context.Background(),
		&apps.Deployment{
			ObjectMeta: meta.ObjectMeta{
				Name: "test",
			},
			Spec: apps.DeploymentSpec{
				Replicas: pointer.Int32Ptr(1),
			},
		},
		meta.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	err = scale.PatchReplicasToScale(
		context.Background(),
		"test",
		3,
		scale.DeploymentScalePatcher(s.client.AppsV1().Deployments("test")),
	)
	c.Assert(err, tc.ErrorIsNil)

	dep, err := s.client.AppsV1().Deployments("test").Get(
		context.Background(),
		"test",
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*dep.Spec.Replicas, tc.Equals, int32(3))

	err = scale.PatchReplicasToScale(
		context.Background(),
		"test",
		0,
		scale.DeploymentScalePatcher(s.client.AppsV1().Deployments("test")),
	)
	c.Assert(err, tc.ErrorIsNil)

	dep, err = s.client.AppsV1().Deployments("test").Get(
		context.Background(),
		"test",
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*dep.Spec.Replicas, tc.Equals, int32(0))
}

func (s *ScaleSuite) TestDeploymentScaleNotFound(c *tc.C) {
	err := scale.PatchReplicasToScale(
		context.Background(),
		"test",
		3,
		scale.DeploymentScalePatcher(s.client.AppsV1().Deployments("test")),
	)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *ScaleSuite) TestStatefulSetScaleNotFound(c *tc.C) {
	err := scale.PatchReplicasToScale(
		context.Background(),
		"test",
		3,
		scale.StatefulSetScalePatcher(s.client.AppsV1().StatefulSets("test")),
	)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *ScaleSuite) TestStatefulSetScale(c *tc.C) {
	_, err := s.client.AppsV1().StatefulSets("test").Create(
		context.Background(),
		&apps.StatefulSet{
			ObjectMeta: meta.ObjectMeta{
				Name: "test",
			},
			Spec: apps.StatefulSetSpec{
				Replicas: pointer.Int32Ptr(1),
			},
		},
		meta.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	err = scale.PatchReplicasToScale(
		context.Background(),
		"test",
		3,
		scale.StatefulSetScalePatcher(s.client.AppsV1().StatefulSets("test")),
	)
	c.Assert(err, tc.ErrorIsNil)

	ss, err := s.client.AppsV1().StatefulSets("test").Get(
		context.Background(),
		"test",
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*ss.Spec.Replicas, tc.Equals, int32(3))

	err = scale.PatchReplicasToScale(
		context.Background(),
		"test",
		0,
		scale.StatefulSetScalePatcher(s.client.AppsV1().StatefulSets("test")),
	)
	c.Assert(err, tc.ErrorIsNil)

	ss, err = s.client.AppsV1().StatefulSets("test").Get(
		context.Background(),
		"test",
		meta.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*ss.Spec.Replicas, tc.Equals, int32(0))
}

func (s *ScaleSuite) TestInvalidScale(c *tc.C) {
	err := scale.PatchReplicasToScale(
		context.Background(),
		"test",
		-1,
		scale.StatefulSetScalePatcher(s.client.AppsV1().StatefulSets("test")),
	)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}
