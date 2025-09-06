// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/storage/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
)

type storageClassSuite struct {
	resourceSuite
	storageClassClient v1.StorageClassInterface
}

var _ = gc.Suite(&storageClassSuite{})

func (s *storageClassSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.storageClassClient = s.client.StorageV1().StorageClasses()
}

func (s *storageClassSuite) TestApply(c *gc.C) {
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sc1",
		},
	}
	// Create.
	scResource := resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "sc1", sc)
	c.Assert(scResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.StorageV1().StorageClasses().Get(context.TODO(), "sc1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	sc.SetAnnotations(map[string]string{"a": "b"})
	scResource = resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "sc1", sc)
	c.Assert(scResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.StorageV1().StorageClasses().Get(context.TODO(), "sc1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `sc1`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *storageClassSuite) TestGet(c *gc.C) {
	template := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sc1",
		},
	}
	sc1 := template
	sc1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.StorageV1().StorageClasses().Create(context.TODO(), &sc1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	scResource := resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "sc1", &template)
	c.Assert(len(scResource.GetAnnotations()), gc.Equals, 0)
	err = scResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scResource.GetName(), gc.Equals, `sc1`)
	c.Assert(scResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *storageClassSuite) TestDelete(c *gc.C) {
	sc := storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sc1",
		},
	}
	_, err := s.client.StorageV1().StorageClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.StorageV1().StorageClasses().Get(context.TODO(), "sc1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `sc1`)

	scResource := resources.NewStorageClass(s.client.StorageV1().StorageClasses(), "sc1", &sc)
	err = scResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = scResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = scResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.StorageV1().StorageClasses().Get(context.TODO(), "sc1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *storageClassSuite) TestListStorageClasses(c *gc.C) {
	// Set up labels for model and app to list resource
	controllerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

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
	_, err = s.storageClassClient.Create(context.TODO(), sc1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create sc2
	sc2Name := "sc2"
	sc2 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:   sc2Name,
			Labels: labelSet,
		},
	}
	_, err = s.storageClassClient.Create(context.TODO(), sc2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	sces, err := resources.ListStorageClass(context.Background(), s.storageClassClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(sces), gc.Equals, 2)
	c.Assert(sces[0].GetName(), gc.Equals, sc1Name)
	c.Assert(sces[1].GetName(), gc.Equals, sc2Name)

	// List resources with no labels.
	sces, err = resources.ListStorageClass(context.Background(), s.storageClassClient, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(sces), gc.Equals, 2)

	// List resources with wrong labels.
	sces, err = resources.ListStorageClass(context.Background(), s.storageClassClient, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(sces), gc.Equals, 0)
}
