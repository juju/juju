// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type persistentVolumeClaimSuite struct {
	resourceSuite
}

var _ = gc.Suite(&persistentVolumeClaimSuite{})

func (s *persistentVolumeClaimSuite) TestApply(c *gc.C) {
	ds := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	// Create.
	dsResource := resources.NewPersistentVolumeClaim("ds1", "test", ds)
	c.Assert(dsResource.Apply(context.TODO(), s.coreClient), jc.ErrorIsNil)
	result, err := s.coreClient.CoreV1().PersistentVolumeClaims("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	ds.SetAnnotations(map[string]string{"a": "b"})
	dsResource = resources.NewPersistentVolumeClaim("ds1", "test", ds)
	c.Assert(dsResource.Apply(context.TODO(), s.coreClient), jc.ErrorIsNil)

	result, err = s.coreClient.CoreV1().PersistentVolumeClaims("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *persistentVolumeClaimSuite) TestGet(c *gc.C) {
	template := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	ds1 := template
	ds1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.coreClient.CoreV1().PersistentVolumeClaims("test").Create(context.TODO(), &ds1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	dsResource := resources.NewPersistentVolumeClaim("ds1", "test", &template)
	c.Assert(len(dsResource.GetAnnotations()), gc.Equals, 0)
	err = dsResource.Get(context.TODO(), s.coreClient)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dsResource.GetName(), gc.Equals, `ds1`)
	c.Assert(dsResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(dsResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *persistentVolumeClaimSuite) TestDelete(c *gc.C) {
	ds := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds1",
			Namespace: "test",
		},
	}
	_, err := s.coreClient.CoreV1().PersistentVolumeClaims("test").Create(context.TODO(), &ds, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.coreClient.CoreV1().PersistentVolumeClaims("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `ds1`)

	dsResource := resources.NewPersistentVolumeClaim("ds1", "test", &ds)
	err = dsResource.Delete(context.TODO(), s.coreClient)
	c.Assert(err, jc.ErrorIsNil)

	err = dsResource.Get(context.TODO(), s.coreClient)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.coreClient.CoreV1().PersistentVolumeClaims("test").Get(context.TODO(), "ds1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *persistentVolumeClaimSuite) TestList(c *gc.C) {
	// Unfortunately with the K8s fake/testing API there doesn't seem to be a
	// way to call List multiple times with "Continue" set.

	// Create fake persistent volume claims, some of which have a label
	for i := 0; i < 7; i++ {
		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pvc%d", i),
				Namespace: "test",
			},
		}
		if i%3 == 0 {
			pvc.ObjectMeta.Labels = map[string]string{"modulo": "three"}
		}
		_, err := s.coreClient.CoreV1().PersistentVolumeClaims("test").Create(context.Background(), &pvc, metav1.CreateOptions{})
		c.Assert(err, jc.ErrorIsNil)
	}

	// List PVCs filtered by the label
	listed, err := resources.ListPersistentVolumeClaims(context.Background(), s.coreClient, "test", metav1.ListOptions{
		LabelSelector: "modulo == three",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we fetch the right ones
	c.Assert(len(listed), gc.Equals, 3)
	for i, pvc := range listed {
		c.Assert(pvc.Name, gc.Equals, fmt.Sprintf("pvc%d", i*3))
		c.Assert(pvc.Labels, gc.DeepEquals, map[string]string{"modulo": "three"})
	}
}
