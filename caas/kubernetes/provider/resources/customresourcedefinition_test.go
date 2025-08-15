// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	providerutils "github.com/juju/juju/caas/kubernetes/provider/utils"
)

type customresourcedefinitionSuite struct {
	resourceSuite
}

var _ = gc.Suite(&customresourcedefinitionSuite{})

func (s *customresourcedefinitionSuite) TestApply(c *gc.C) {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "crd1",
		},
	}
	// Create.
	crdResource := resources.NewCustomResourceDefinition(s.extendedClient.ApiextensionsV1().CustomResourceDefinitions(), "crd1", crd)
	c.Assert(crdResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), "crd1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	crd.SetAnnotations(map[string]string{"a": "b"})
	crdResource = resources.NewCustomResourceDefinition(s.extendedClient.ApiextensionsV1().CustomResourceDefinitions(), "crd1", crd)
	c.Assert(crdResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), "crd1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `crd1`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *customresourcedefinitionSuite) TestGet(c *gc.C) {
	template := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "crd1",
		},
	}
	crd1 := template
	crd1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), &crd1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	crdResource := resources.NewCustomResourceDefinition(s.extendedClient.ApiextensionsV1().CustomResourceDefinitions(), "crd1", &template)
	c.Assert(len(crdResource.GetAnnotations()), gc.Equals, 0)
	err = crdResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(crdResource.GetName(), gc.Equals, `crd1`)
	c.Assert(crdResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *customresourcedefinitionSuite) TestDelete(c *gc.C) {
	crd := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "crd1",
		},
	}
	_, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), &crd, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), "crd1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `crd1`)

	crdResource := resources.NewCustomResourceDefinition(s.extendedClient.ApiextensionsV1().CustomResourceDefinitions(), "crd1", &crd)
	err = crdResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = crdResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), "crd1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *customresourcedefinitionSuite) TestListCRDs(c *gc.C) {
	controllerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelName := "testmodel"

	appName := "app1"
	appLabel := providerutils.SelectorLabelsForApp(appName, constants.LabelVersion2)

	modelLabel := providerutils.LabelsForModel(modelName, modelUUID.String(), controllerUUID.String(), constants.LabelVersion2)
	labelSet := providerutils.LabelsMerge(appLabel, modelLabel)

	crd1Name := "crd1"
	crd1 := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crd1Name,
			Labels: labelSet,
		},
	}
	_, err = s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), &crd1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	crd2Name := "crd2"
	crd2 := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crd2Name,
			Labels: labelSet,
		},
	}
	_, err = s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), &crd2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	crds, err := resources.ListCRDs(context.Background(), s.extendedClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(crds), gc.Equals, 2)
	c.Assert(crds[0].GetName(), gc.Equals, crd1Name)
	c.Assert(crds[1].GetName(), gc.Equals, crd2Name)
}
