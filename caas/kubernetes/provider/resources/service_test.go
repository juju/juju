// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type serviceSuite struct {
	resourceSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestApply(c *gc.C) {
	ds := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	// Create.
	dsResource := resources.NewService(s.client.CoreV1().Services(ds.Namespace), "test", "ds1", ds)
	c.Assert(dsResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.CoreV1().Services("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	ds.SetAnnotations(map[string]string{"a": "b"})
	dsResource = resources.NewService(s.client.CoreV1().Services(ds.Namespace), "test", "ds1", ds)
	c.Assert(dsResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.CoreV1().Services("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *serviceSuite) TestGet(c *gc.C) {
	template := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	ds1 := template
	ds1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.CoreV1().Services("test").Create(context.TODO(), &ds1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	dsResource := resources.NewService(s.client.CoreV1().Services(ds1.Namespace), "test", "ds1", &template)
	c.Assert(len(dsResource.GetAnnotations()), gc.Equals, 0)
	err = dsResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dsResource.GetName(), gc.Equals, `ds1`)
	c.Assert(dsResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(dsResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *serviceSuite) TestDelete(c *gc.C) {
	ds := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	_, err := s.client.CoreV1().Services("test").Create(context.TODO(), &ds, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.CoreV1().Services("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)

	dsResource := resources.NewService(s.client.CoreV1().Services(ds.Namespace), "test", "ds1", &ds)
	err = dsResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = dsResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.CoreV1().Services("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}
