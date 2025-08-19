// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/uuid"
)

type customresourcedefinitionSuite struct {
	resourceSuite
}

func TestCustomResourceDefinitionSuite(t *testing.T) {
	tc.Run(t, &customresourcedefinitionSuite{})
}

func (s *customresourcedefinitionSuite) TestApply(c *tc.C) {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "crd1",
		},
	}
	// Create.
	crdResource := resources.NewCustomResourceDefinition(s.extendedClient.ApiextensionsV1().CustomResourceDefinitions(), "crd1", crd)
	c.Assert(crdResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Get(c.Context(), "crd1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	crd.SetAnnotations(map[string]string{"a": "b"})
	crdResource = resources.NewCustomResourceDefinition(s.extendedClient.ApiextensionsV1().CustomResourceDefinitions(), "crd1", crd)
	c.Assert(crdResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Get(c.Context(), "crd1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `crd1`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *customresourcedefinitionSuite) TestGet(c *tc.C) {
	template := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "crd1",
		},
	}
	crd1 := template
	crd1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Create(c.Context(), &crd1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	crdResource := resources.NewCustomResourceDefinition(s.extendedClient.ApiextensionsV1().CustomResourceDefinitions(), "crd1", &template)
	c.Assert(len(crdResource.GetAnnotations()), tc.Equals, 0)
	err = crdResource.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(crdResource.GetName(), tc.Equals, `crd1`)
	c.Assert(crdResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *customresourcedefinitionSuite) TestDelete(c *tc.C) {
	crd := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "crd1",
		},
	}
	_, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Create(c.Context(), &crd, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Get(c.Context(), "crd1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `crd1`)

	crdResource := resources.NewCustomResourceDefinition(s.extendedClient.ApiextensionsV1().CustomResourceDefinitions(), "crd1", &crd)
	err = crdResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = crdResource.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Get(c.Context(), "crd1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *customresourcedefinitionSuite) TestListCRDs(c *tc.C) {
	controllerUUID := uuid.MustNewUUID()
	modelUUID := uuid.MustNewUUID()

	modelName := "testmodel"

	appName := "app1"
	appLabel := utils.SelectorLabelsForApp(appName, constants.LabelVersion2)

	modelLabel := utils.LabelsForModel(modelName, modelUUID.String(), controllerUUID.String(), constants.LabelVersion2)
	labelSet := utils.LabelsMerge(appLabel, modelLabel)

	crd1Name := "crd1"
	crd1 := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crd1Name,
			Labels: labelSet,
		},
	}
	_, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Create(c.Context(), &crd1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	crd2Name := "crd2"
	crd2 := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   crd2Name,
			Labels: labelSet,
		},
	}
	_, err = s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Create(c.Context(), &crd2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	crds, err := resources.ListCRDs(context.Background(), s.extendedClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(crds), tc.Equals, 2)
	c.Assert(crds[0].GetName(), tc.Equals, crd1Name)
	c.Assert(crds[1].GetName(), tc.Equals, crd2Name)
}
