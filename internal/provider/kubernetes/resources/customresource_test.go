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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
)

type customresourceSuite struct {
	resourceSuite
	dynamicClient dynamic.Interface
}

var _ = gc.Suite(&customresourceSuite{})

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

func (s *customresourceSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	scheme := runtime.NewScheme()

	gv := schema.GroupVersion{Group: "example.com", Version: "v1"}

	// Register the unstructured types so the fake can construct list objects.
	scheme.AddKnownTypeWithName(gv.WithKind("Widget"), &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(gv.WithKind("WidgetList"), &unstructured.UnstructuredList{})

	s.dynamicClient = dynamicfake.NewSimpleDynamicClient(scheme)
}

func (s *customresourceSuite) TestApply(c *gc.C) {
	client := s.getClusterDynamicClient(schema.GroupVersionResource{
		Group:    "example.com",
		Version:  "v1",
		Resource: "widgets",
	})

	cr := makeCR("cr1", "", nil)

	// Create.
	crResource := resources.NewCustomResource(client, "cr1", &cr)
	c.Assert(crResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := client.Get(context.TODO(), "cr1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	cr.SetAnnotations(map[string]string{"a": "b"})
	crResource = resources.NewCustomResource(client, "cr1", &cr)
	c.Assert(crResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = client.Get(context.TODO(), "cr1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `cr1`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *customresourceSuite) TestGet(c *gc.C) {
	client := s.getClusterDynamicClient(schema.GroupVersionResource{
		Group:    "example.com",
		Version:  "v1",
		Resource: "widgets",
	})

	template := makeCR("cr1", "", nil)

	cr1 := template.DeepCopy()
	cr1.SetAnnotations(map[string]string{"a": "b"})
	_, err := client.Create(context.TODO(), cr1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	crResource := resources.NewCustomResource(client, "cr1", &template)
	c.Assert(len(crResource.GetAnnotations()), gc.Equals, 0)

	err = crResource.Get(context.TODO())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(crResource.GetName(), gc.Equals, `cr1`)
	c.Assert(crResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *customresourceSuite) TestDelete(c *gc.C) {
	client := s.getClusterDynamicClient(schema.GroupVersionResource{
		Group:    "example.com",
		Version:  "v1",
		Resource: "widgets",
	})
	cr := makeCR("cr1", "", nil)
	_, err := client.Create(context.TODO(), &cr, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := client.Get(context.TODO(), "cr1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `cr1`)

	crResource := resources.NewCustomResource(client, "cr1", &cr)
	err = crResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = crResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = crResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = client.Get(context.TODO(), "cr1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *customresourceSuite) TestListCRsForCRD(c *gc.C) {
	controllerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(crdResource.Apply(context.TODO()), jc.ErrorIsNil)
	crd1, err := s.extendedClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), "crd1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)

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
	_, err = crNamespacedClient.Create(context.TODO(), &cr1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create namespaced cr2 with crd.
	cr2Name := "cr2"
	cr2 := makeCR(cr2Name, ns, labelSet)
	_, err = crNamespacedClient.Create(context.TODO(), &cr2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	crs, err := resources.ListCRsForCRD(context.Background(), s.dynamicClient, ns, crd1, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(crs), gc.Equals, 2)
	c.Assert(crs[0].GetName(), gc.Equals, cr1Name)
	c.Assert(crs[1].GetName(), gc.Equals, cr2Name)

	// List resources with empty labels.
	crs, err = resources.ListCRsForCRD(context.Background(), s.dynamicClient, ns, crd1, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(crs), gc.Equals, 2)

	// List resources with incorrect labels.
	crs, err = resources.ListCRsForCRD(context.Background(), s.dynamicClient, ns, crd1, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(crs), gc.Equals, 0)
}
