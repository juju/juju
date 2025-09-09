// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/internal/provider/kubernetes/resources"
)

type persistentVolumeSuite struct {
	resourceSuite
}

func TestPersistentVolumeSuite(t *testing.T) {
	tc.Run(t, &persistentVolumeSuite{})
}

func (s *persistentVolumeSuite) TestApply(c *tc.C) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv1",
		},
	}
	// Create.
	pvResource := resources.NewPersistentVolume(s.client.CoreV1().PersistentVolumes(), "pv1", pv)
	c.Assert(pvResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.client.CoreV1().PersistentVolumes().Get(c.Context(), "pv1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	pv.SetAnnotations(map[string]string{"a": "b"})
	pvResource = resources.NewPersistentVolume(s.client.CoreV1().PersistentVolumes(), "pv1", pv)
	c.Assert(pvResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = s.client.CoreV1().PersistentVolumes().Get(c.Context(), "pv1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `pv1`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *persistentVolumeSuite) TestGet(c *tc.C) {
	template := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv1",
		},
	}
	pv1 := template
	pv1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.CoreV1().PersistentVolumes().Create(c.Context(), &pv1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	pvResource := resources.NewPersistentVolume(s.client.CoreV1().PersistentVolumes(), "pv1", &template)
	c.Assert(len(pvResource.GetAnnotations()), tc.Equals, 0)
	err = pvResource.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pvResource.GetName(), tc.Equals, `pv1`)
	c.Assert(pvResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *persistentVolumeSuite) TestDelete(c *tc.C) {
	pv := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv1",
		},
	}
	_, err := s.client.CoreV1().PersistentVolumes().Create(c.Context(), &pv, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.CoreV1().PersistentVolumes().Get(c.Context(), "pv1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `pv1`)

	pvResource := resources.NewPersistentVolume(s.client.CoreV1().PersistentVolumes(), "pv1", &pv)
	err = pvResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = pvResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = pvResource.Get(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	_, err = s.client.CoreV1().PersistentVolumes().Get(c.Context(), "pv1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}
