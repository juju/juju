// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type statefulSetSuite struct {
	resourceSuite
}

var _ = tc.Suite(&statefulSetSuite{})

func (s *statefulSetSuite) TestApply(c *tc.C) {
	ds := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	// Create.
	dsResource := resources.NewStatefulSet("ds1", "test", ds)
	c.Assert(dsResource.Apply(context.Background(), s.client), tc.ErrorIsNil)
	result, err := s.client.AppsV1().StatefulSets("test").Get(context.Background(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	ds.SetAnnotations(map[string]string{"a": "b"})
	dsResource = resources.NewStatefulSet("ds1", "test", ds)
	c.Assert(dsResource.Apply(context.Background(), s.client), tc.ErrorIsNil)

	result, err = s.client.AppsV1().StatefulSets("test").Get(context.Background(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `ds1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *statefulSetSuite) TestGet(c *tc.C) {
	template := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	ds1 := template
	ds1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.AppsV1().StatefulSets("test").Create(context.Background(), &ds1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	dsResource := resources.NewStatefulSet("ds1", "test", &template)
	c.Assert(len(dsResource.GetAnnotations()), tc.Equals, 0)
	err = dsResource.Get(context.Background(), s.client)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dsResource.GetName(), tc.Equals, `ds1`)
	c.Assert(dsResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(dsResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *statefulSetSuite) TestDelete(c *tc.C) {
	ds := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	_, err := s.client.AppsV1().StatefulSets("test").Create(context.Background(), &ds, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.AppsV1().StatefulSets("test").Get(context.Background(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `ds1`)

	dsResource := resources.NewStatefulSet("ds1", "test", &ds)
	err = dsResource.Delete(context.Background(), s.client)
	c.Assert(err, tc.ErrorIsNil)

	err = dsResource.Get(context.Background(), s.client)
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	_, err = s.client.AppsV1().StatefulSets("test").Get(context.Background(), "ds1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}
