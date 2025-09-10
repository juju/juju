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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/uuid"
)

type customresourceSuite struct {
	resourceSuite
	dynamicClient dynamic.Interface
}

func TestCustomResourceSuite(t *testing.T) {
	tc.Run(t, &customresourceSuite{})
}

func makeCR(name, namespace string, labels map[string]string) unstructured.Unstructured {
	obj := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "example.com/v1",
			"kind":       "Widget",
			"metadata": map[string]any{
				"name": name,
			},
			"spec": map[string]any{"size": "M"},
		},
	}
	if namespace != "" {
		obj.SetNamespace(namespace)
	}
	if labels != nil {
		obj.SetLabels(labels)
	}
	return obj
}

func (s *customresourceSuite) getClusterDynamicClient(gvr schema.GroupVersionResource) dynamic.ResourceInterface {
	return s.dynamicClient.Resource(gvr)
}

func (s *customresourceSuite) getNamespacedDynamicClient(gvr schema.GroupVersionResource, namespace string) dynamic.ResourceInterface {
	return s.dynamicClient.Resource(gvr).Namespace(namespace)
}

func (s *customresourceSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	scheme := runtime.NewScheme()

	gv := schema.GroupVersion{Group: "example.com", Version: "v1"}

	// Register the unstructured types so the fake can construct list objects.
	scheme.AddKnownTypeWithName(gv.WithKind("Widget"), &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(gv.WithKind("WidgetList"), &unstructured.UnstructuredList{})

	s.dynamicClient = dynamicfake.NewSimpleDynamicClient(scheme)
}

func (s *customresourceSuite) TestApply(c *tc.C) {
	client := s.getClusterDynamicClient(schema.GroupVersionResource{
		Group:    "example.com",
		Version:  "v1",
		Resource: "widgets",
	})

	cr := makeCR("cr1", "", nil)

	// Create.
	crResource := resources.NewCustomResource(client, "cr1", &cr)
	c.Assert(crResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := client.Get(c.Context(), "cr1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	cr.SetAnnotations(map[string]string{"a": "b"})
	crResource = resources.NewCustomResource(client, "cr1", &cr)
	c.Assert(crResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = client.Get(c.Context(), "cr1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `cr1`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *customresourceSuite) TestGet(c *tc.C) {
	client := s.getClusterDynamicClient(schema.GroupVersionResource{
		Group:    "example.com",
		Version:  "v1",
		Resource: "widgets",
	})

	template := makeCR("cr1", "", nil)

	cr1 := template.DeepCopy()
	cr1.SetAnnotations(map[string]string{"a": "b"})
	_, err := client.Create(c.Context(), cr1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	crResource := resources.NewCustomResource(client, "cr1", &template)
	c.Assert(len(crResource.GetAnnotations()), tc.Equals, 0)

	err = crResource.Get(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(crResource.GetName(), tc.Equals, `cr1`)
	c.Assert(crResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *customresourceSuite) TestDelete(c *tc.C) {
	client := s.getClusterDynamicClient(schema.GroupVersionResource{
		Group:    "example.com",
		Version:  "v1",
		Resource: "widgets",
	})
	cr := makeCR("cr1", "", nil)
	_, err := client.Create(c.Context(), &cr, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := client.Get(c.Context(), "cr1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `cr1`)

	crResource := resources.NewCustomResource(client, "cr1", &cr)
	err = crResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = crResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = crResource.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = client.Get(c.Context(), "cr1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *customresourceSuite) TestListCRsForCRD(c *tc.C) {
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelName := "testmodel"

	appName := "app1"
	appLabel := providerutils.SelectorLabelsForApp(appName, constants.LabelVersion2)

	modelLabel := providerutils.LabelsForModel(modelName, modelUUID.String(), controllerUUID.String(), constants.LabelVersion2)
	labelSet := providerutils.LabelsMerge(appLabel, modelLabel)

	// Create CRD.
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "crd1"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "widgets",
				Singular: "widget",
				Kind:     "Widget",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    "v1",
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"size": {Type: "string"},
								},
							},
							"status": {Type: "object"},
						},
					},
				},
			}},
		},
	}
	crdResource := resources.NewCustomResourceDefinition(s.extendedClient.ApiextensionsV1().CustomResourceDefinitions(), "crd1", crd)
	c.Assert(crdResource.Apply(c.Context()), tc.ErrorIsNil)
	crd1, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Get(c.Context(), "crd1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create namespaced cr with crd.
	ns := "ns1"
	cr1Name := "cr1"
	cr1 := makeCR(cr1Name, ns, labelSet)

	gvr := schema.GroupVersionResource{
		Group:    "example.com",
		Version:  "v1",
		Resource: "widgets",
	}
	crNamespacedClient := s.getNamespacedDynamicClient(gvr, ns)

	// Create namespaced cr1 with crd.
	_, err = crNamespacedClient.Create(c.Context(), &cr1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create namespaced cr2 with crd.
	cr2Name := "cr2"
	cr2 := makeCR(cr2Name, ns, labelSet)
	_, err = crNamespacedClient.Create(c.Context(), &cr2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	crs, err := resources.ListCRsForCRD(context.Background(), s.dynamicClient, ns, crd1, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(crs), tc.Equals, 2)
	c.Assert(crs[0].GetName(), tc.Equals, cr1Name)
	c.Assert(crs[1].GetName(), tc.Equals, cr2Name)

	// List resources with empty labels.
	crs, err = resources.ListCRsForCRD(context.Background(), s.dynamicClient, ns, crd1, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(crs), tc.Equals, 2)

	// List resources with incorrect labels.
	crs, err = resources.ListCRsForCRD(context.Background(), s.dynamicClient, ns, crd1, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(crs), tc.Equals, 0)
}
