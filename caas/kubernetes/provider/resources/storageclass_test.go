// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type storageClassSuite struct {
	resourceSuite
}

var _ = gc.Suite(&storageClassSuite{})

func (s *storageClassSuite) TestApply(c *gc.C) {
	ds := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ds1",
		},
	}
	// Create.
	dsResource := resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "ds1", ds)
	c.Assert(dsResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.StorageV1().StorageClasses().Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	ds.SetAnnotations(map[string]string{"a": "b"})
	dsResource = resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "ds1", ds)
	c.Assert(dsResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.StorageV1().StorageClasses().Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *storageClassSuite) TestGet(c *gc.C) {
	template := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ds1",
		},
	}
	ds1 := template
	ds1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.StorageV1().StorageClasses().Create(context.TODO(), &ds1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	dsResource := resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "ds1", &template)
	c.Assert(len(dsResource.GetAnnotations()), gc.Equals, 0)
	err = dsResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dsResource.GetName(), gc.Equals, `ds1`)
	c.Assert(dsResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *storageClassSuite) TestDelete(c *gc.C) {
	ds := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ds1",
		},
	}
	_, err := s.client.StorageV1().StorageClasses().Create(context.TODO(), &ds, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.StorageV1().StorageClasses().Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)

	dsResource := resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "ds1", &ds)
	err = dsResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = dsResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.StorageV1().StorageClasses().Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}
