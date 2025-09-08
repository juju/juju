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

	"github.com/juju/juju/internal/provider/kubernetes/resources"
)

type persistentVolumeSuite struct {
	resourceSuite
}

var _ = gc.Suite(&persistentVolumeSuite{})

func (s *persistentVolumeSuite) TestApply(c *gc.C) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv1",
		},
	}
	// Create.
	pvResource := resources.NewPersistentVolume(s.client.CoreV1().PersistentVolumes(), "pv1", pv)
	c.Assert(pvResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.CoreV1().PersistentVolumes().Get(context.TODO(), "pv1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	pv.SetAnnotations(map[string]string{"a": "b"})
	pvResource = resources.NewPersistentVolume(s.client.CoreV1().PersistentVolumes(), "pv1", pv)
	c.Assert(pvResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.CoreV1().PersistentVolumes().Get(context.TODO(), "pv1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `pv1`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *persistentVolumeSuite) TestGet(c *gc.C) {
	template := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv1",
		},
	}
	pv1 := template
	pv1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.CoreV1().PersistentVolumes().Create(context.TODO(), &pv1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	pvResource := resources.NewPersistentVolume(s.client.CoreV1().PersistentVolumes(), "pv1", &template)
	c.Assert(len(pvResource.GetAnnotations()), gc.Equals, 0)
	err = pvResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pvResource.GetName(), gc.Equals, `pv1`)
	c.Assert(pvResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *persistentVolumeSuite) TestDelete(c *gc.C) {
	pv := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv1",
		},
	}
	_, err := s.client.CoreV1().PersistentVolumes().Create(context.TODO(), &pv, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.CoreV1().PersistentVolumes().Get(context.TODO(), "pv1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `pv1`)

	pvResource := resources.NewPersistentVolume(s.client.CoreV1().PersistentVolumes(), "pv1", &pv)
	err = pvResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = pvResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = pvResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.CoreV1().PersistentVolumes().Get(context.TODO(), "pv1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}
