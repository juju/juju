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

	"github.com/juju/juju/internal/provider/caas/kubernetes/provider/resources"
)

type daemonsetSuite struct {
	resourceSuite
}

var _ = gc.Suite(&daemonsetSuite{})

func (s *daemonsetSuite) TestApply(c *gc.C) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	// Create.
	dsResource := resources.NewDaemonSet("ds1", "test", ds)
	c.Assert(dsResource.Apply(context.Background(), s.client), jc.ErrorIsNil)
	result, err := s.client.AppsV1().DaemonSets("test").Get(context.Background(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	ds.SetAnnotations(map[string]string{"a": "b"})
	dsResource = resources.NewDaemonSet("ds1", "test", ds)
	c.Assert(dsResource.Apply(context.Background(), s.client), jc.ErrorIsNil)

	result, err = s.client.AppsV1().DaemonSets("test").Get(context.Background(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *daemonsetSuite) TestGet(c *gc.C) {
	template := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	ds1 := template
	ds1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.AppsV1().DaemonSets("test").Create(context.Background(), &ds1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	dsResource := resources.NewDaemonSet("ds1", "test", &template)
	c.Assert(len(dsResource.GetAnnotations()), gc.Equals, 0)
	err = dsResource.Get(context.Background(), s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dsResource.GetName(), gc.Equals, `ds1`)
	c.Assert(dsResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(dsResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *daemonsetSuite) TestDelete(c *gc.C) {
	ds := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	_, err := s.client.AppsV1().DaemonSets("test").Create(context.Background(), &ds, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.AppsV1().DaemonSets("test").Get(context.Background(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)

	dsResource := resources.NewDaemonSet("ds1", "test", &ds)
	err = dsResource.Delete(context.Background(), s.client)
	c.Assert(err, jc.ErrorIsNil)

	err = dsResource.Get(context.Background(), s.client)
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	_, err = s.client.AppsV1().DaemonSets("test").Get(context.Background(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}
