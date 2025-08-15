// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type deploymentSuite struct {
	resourceSuite
}

var _ = gc.Suite(&deploymentSuite{})

func (s *deploymentSuite) TestApply(c *gc.C) {
	ds := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	// Create.
	dsResource := resources.NewDeployment(s.client.AppsV1().Deployments(ds.Namespace), "test", "ds1", ds)
	c.Assert(dsResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.AppsV1().Deployments("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	ds.SetAnnotations(map[string]string{"a": "b"})
	dsResource = resources.NewDeployment(s.client.AppsV1().Deployments(ds.Namespace), "test", "ds1", ds)
	c.Assert(dsResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.AppsV1().Deployments("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *deploymentSuite) TestGet(c *gc.C) {
	template := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	ds1 := template
	ds1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.AppsV1().Deployments("test").Create(context.TODO(), &ds1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	dsResource := resources.NewDeployment(s.client.AppsV1().Deployments(ds1.Namespace), "test", "ds1", &template)
	c.Assert(len(dsResource.GetAnnotations()), gc.Equals, 0)
	err = dsResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dsResource.GetName(), gc.Equals, `ds1`)
	c.Assert(dsResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(dsResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *deploymentSuite) TestDelete(c *gc.C) {
	ds := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	_, err := s.client.AppsV1().Deployments("test").Create(context.TODO(), &ds, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.AppsV1().Deployments("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)

	dsResource := resources.NewDeployment(s.client.AppsV1().Deployments(ds.Namespace), "test", "ds1", &ds)
	err = dsResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = dsResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.AppsV1().Deployments("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}
