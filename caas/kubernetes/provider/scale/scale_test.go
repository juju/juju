// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scale_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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

var _ = gc.Suite(&ScaleSuite{})

func (s *ScaleSuite) SetUpTest(c *gc.C) {
	s.client = fake.NewSimpleClientset()
	_, err := s.client.CoreV1().Namespaces().Create(
		context.TODO(),
		&core.Namespace{
			ObjectMeta: meta.ObjectMeta{
				Name: "test",
			},
		},
		meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ScaleSuite) TestDeploymentScale(c *gc.C) {
	_, err := s.client.AppsV1().Deployments("test").Create(
		context.TODO(),
		&apps.Deployment{
			ObjectMeta: meta.ObjectMeta{
				Name: "test",
			},
			Spec: apps.DeploymentSpec{
				Replicas: pointer.Int32Ptr(1),
			},
		},
		meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	err = scale.PatchReplicasToScale(
		context.TODO(),
		"test",
		3,
		scale.DeploymentScalePatcher(s.client.AppsV1().Deployments("test")),
	)
	c.Assert(err, jc.ErrorIsNil)

	dep, err := s.client.AppsV1().Deployments("test").Get(
		context.TODO(),
		"test",
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*dep.Spec.Replicas, gc.Equals, int32(3))

	err = scale.PatchReplicasToScale(
		context.TODO(),
		"test",
		0,
		scale.DeploymentScalePatcher(s.client.AppsV1().Deployments("test")),
	)
	c.Assert(err, jc.ErrorIsNil)

	dep, err = s.client.AppsV1().Deployments("test").Get(
		context.TODO(),
		"test",
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*dep.Spec.Replicas, gc.Equals, int32(0))
}

func (s *ScaleSuite) TestDeploymentScaleNotFound(c *gc.C) {
	err := scale.PatchReplicasToScale(
		context.TODO(),
		"test",
		3,
		scale.DeploymentScalePatcher(s.client.AppsV1().Deployments("test")),
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ScaleSuite) TestStatefulSetScaleNotFound(c *gc.C) {
	err := scale.PatchReplicasToScale(
		context.TODO(),
		"test",
		3,
		scale.StatefulSetScalePatcher(s.client.AppsV1().StatefulSets("test")),
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *ScaleSuite) TestStatefulSetScale(c *gc.C) {
	_, err := s.client.AppsV1().StatefulSets("test").Create(
		context.TODO(),
		&apps.StatefulSet{
			ObjectMeta: meta.ObjectMeta{
				Name: "test",
			},
			Spec: apps.StatefulSetSpec{
				Replicas: pointer.Int32Ptr(1),
			},
		},
		meta.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	err = scale.PatchReplicasToScale(
		context.TODO(),
		"test",
		3,
		scale.StatefulSetScalePatcher(s.client.AppsV1().StatefulSets("test")),
	)
	c.Assert(err, jc.ErrorIsNil)

	ss, err := s.client.AppsV1().StatefulSets("test").Get(
		context.TODO(),
		"test",
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*ss.Spec.Replicas, gc.Equals, int32(3))

	err = scale.PatchReplicasToScale(
		context.TODO(),
		"test",
		0,
		scale.StatefulSetScalePatcher(s.client.AppsV1().StatefulSets("test")),
	)
	c.Assert(err, jc.ErrorIsNil)

	ss, err = s.client.AppsV1().StatefulSets("test").Get(
		context.TODO(),
		"test",
		meta.GetOptions{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*ss.Spec.Replicas, gc.Equals, int32(0))
}

func (s *ScaleSuite) TestInvalidScale(c *gc.C) {
	err := scale.PatchReplicasToScale(
		context.TODO(),
		"test",
		-1,
		scale.StatefulSetScalePatcher(s.client.AppsV1().StatefulSets("test")),
	)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}
