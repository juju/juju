// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/storage/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/uuid"
)

type storageClassSuite struct {
	resourceSuite
	storageClassClient v1.StorageClassInterface
}

func TestStorageClassSuite(t *testing.T) {
	tc.Run(t, &storageClassSuite{})
}

func (s *storageClassSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.storageClassClient = s.client.StorageV1().StorageClasses()
}

func (s *storageClassSuite) TestApply(c *tc.C) {
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sc1",
		},
	}
	// Create.
	scResource := resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "sc1", sc)
	c.Assert(scResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.client.StorageV1().StorageClasses().Get(c.Context(), "sc1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	sc.SetAnnotations(map[string]string{"a": "b"})
	scResource = resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "sc1", sc)
	c.Assert(scResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = s.client.StorageV1().StorageClasses().Get(c.Context(), "sc1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `sc1`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *storageClassSuite) TestGet(c *tc.C) {
	template := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sc1",
		},
	}
	sc1 := template
	sc1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.StorageV1().StorageClasses().Create(c.Context(), &sc1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	scResource := resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "sc1", &template)
	c.Assert(len(scResource.GetAnnotations()), tc.Equals, 0)
	err = scResource.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scResource.GetName(), tc.Equals, `sc1`)
	c.Assert(scResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *storageClassSuite) TestDelete(c *tc.C) {
	sc := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sc1",
		},
	}
	_, err := s.client.StorageV1().StorageClasses().Create(c.Context(), &sc, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.StorageV1().StorageClasses().Get(c.Context(), "sc1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `sc1`)

	scResource := resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "sc1", &sc)
	err = scResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = scResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = scResource.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.StorageV1().StorageClasses().Get(c.Context(), "sc1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *storageClassSuite) TestListStorageClasses(c *tc.C) {
	// Set up labels for model and app to list resource
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelName := "testmodel"

	appName := "app1"
	appLabel := providerutils.SelectorLabelsForApp(appName, constants.LabelVersion2)

	modelLabel := providerutils.LabelsForModel(modelName, modelUUID.String(), controllerUUID.String(), constants.LabelVersion2)
	labelSet := providerutils.LabelsMerge(appLabel, modelLabel)

	// Create sc1
	sc1Name := "sc1"
	sc1 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:   sc1Name,
			Labels: labelSet,
		},
	}
	_, err = s.storageClassClient.Create(c.Context(), sc1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create sc2
	sc2Name := "sc2"
	sc2 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:   sc2Name,
			Labels: labelSet,
		},
	}
	_, err = s.storageClassClient.Create(c.Context(), sc2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	sces, err := resources.ListStorageClass(context.Background(), s.storageClassClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(sces), tc.Equals, 2)
	c.Assert(sces[0].GetName(), tc.Equals, sc1Name)
	c.Assert(sces[1].GetName(), tc.Equals, sc2Name)

	// List resources with no labels.
	sces, err = resources.ListStorageClass(context.Background(), s.storageClassClient, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(sces), tc.Equals, 2)

	// List resources with wrong labels.
	sces, err = resources.ListStorageClass(context.Background(), s.storageClassClient, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(sces), tc.Equals, 0)
}
