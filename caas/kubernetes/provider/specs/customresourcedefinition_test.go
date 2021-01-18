// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/testing"
)

type crdSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&crdSuite{})

func (s *crdSuite) TestK8sCustomResourceDefinitionV1Beta1(c *gc.C) {
	specV1Beta1 := `
name: tfjobs.kubeflow.org
labels:
  foo: bar
  juju-global-resource-lifecycle: model
spec:
  group: kubeflow.org
  scope: Cluster
  names:
    kind: TFJob
    singular: tfjob
    plural: tfjobs
  version: v1
  versions:
    - name: v1
      served: true
      storage: true
    - name: v1beta2
      served: true
      storage: false
  conversion:
    strategy: None
  preserveUnknownFields: false
  additionalPrinterColumns:
    - name: Worker
      type: integer
      description: Worker attribute.
      jsonPath: .spec.tfReplicaSpecs.Worker
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            tfReplicaSpecs:
              properties:
                Worker:
                  properties:
                    replicas:
                      type: integer
                      minimum: 1
                PS:
                  properties:
                    replicas:
                      type: integer
                      minimum: 1
                Chief:
                  properties:
                    replicas:
                      type: integer
                      minimum: 1
                      maximum: 1
`
	var obj k8sspecs.K8sCustomResourceDefinition
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(specV1Beta1), len(specV1Beta1)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obj, gc.DeepEquals, k8sspecs.K8sCustomResourceDefinition{
		Meta: k8sspecs.Meta{
			Name: "tfjobs.kubeflow.org",
			Labels: map[string]string{
				"foo":                            "bar",
				"juju-global-resource-lifecycle": "model",
			},
		},
		Spec: k8sspecs.K8sCustomResourceDefinitionSpec{
			Version: k8sspecs.K8sCustomResourceDefinitionV1Beta1,
			SpecV1Beta1: apiextensionsv1beta1.CustomResourceDefinitionSpec{
				Group:   "kubeflow.org",
				Version: "v1",
				Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
					{Name: "v1", Served: true, Storage: true},
					{Name: "v1beta2", Served: true, Storage: false},
				},
				Scope:                 "Cluster",
				PreserveUnknownFields: boolPtr(false),
				Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
					Kind:     "TFJob",
					Plural:   "tfjobs",
					Singular: "tfjob",
				},
				Conversion: &apiextensionsv1beta1.CustomResourceConversion{
					Strategy: apiextensionsv1beta1.NoneConverter,
				},
				AdditionalPrinterColumns: []apiextensionsv1beta1.CustomResourceColumnDefinition{
					{
						Name:        "Worker",
						Type:        "integer",
						Description: "Worker attribute.",
						JSONPath:    ".spec.tfReplicaSpecs.Worker",
					},
				},
				Validation: &apiextensionsv1beta1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
						Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
							"spec": {
								Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
									"tfReplicaSpecs": {
										Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
											"PS": {
												Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
													"replicas": {
														Type: "integer", Minimum: float64Ptr(1),
													},
												},
											},
											"Chief": {
												Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
													"replicas": {
														Type:    "integer",
														Minimum: float64Ptr(1),
														Maximum: float64Ptr(1),
													},
												},
											},
											"Worker": {
												Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
													"replicas": {
														Type:    "integer",
														Minimum: float64Ptr(1),
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
}

func (s *crdSuite) TestK8sCustomResourceDefinitionV1(c *gc.C) {
	specV1 := `
name: certificates.networking.internal.knative.dev
labels:
  knative.dev/crd-install: "true"
  serving.knative.dev/release: "v0.19.0"
spec:
  scope: Namespaced
  group: networking.internal.knative.dev
  names:
    kind: Certificate
    plural: certificates
    singular: certificate
    categories:
      - knative-internal
      - networking
    shortNames:
      - kcert
  versions:
    - name: v1alpha1
      served: true
      storage: true
      subresources:
        status: {}
      schema:
        openAPIV3Schema:
          type: object
          # this is a work around so we don't need to flush out the
          # schema for each version at this time
          #
          # see issue: https://github.com/knative/serving/issues/912
          x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
        - name: Ready
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Ready\")].status"
        - name: Reason
          type: string
          jsonPath: ".status.conditions[?(@.type==\"Ready\")].reason"
`
	var obj k8sspecs.K8sCustomResourceDefinition
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(specV1), len(specV1)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obj, gc.DeepEquals, k8sspecs.K8sCustomResourceDefinition{
		Meta: k8sspecs.Meta{
			Name: "certificates.networking.internal.knative.dev",
			Labels: map[string]string{
				"knative.dev/crd-install":     "true",
				"serving.knative.dev/release": "v0.19.0",
			},
		},
		Spec: k8sspecs.K8sCustomResourceDefinitionSpec{
			Version: k8sspecs.K8sCustomResourceDefinitionV1,
			SpecV1: apiextensionsv1.CustomResourceDefinitionSpec{
				Scope: apiextensionsv1.NamespaceScoped,
				Group: "networking.internal.knative.dev",
				Names: apiextensionsv1.CustomResourceDefinitionNames{
					Kind:     "Certificate",
					Plural:   "certificates",
					Singular: "certificate",
					Categories: []string{
						"knative-internal",
						"networking",
					},
					ShortNames: []string{
						"kcert",
					},
				},
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
					{
						Name:    "v1alpha1",
						Served:  true,
						Storage: true,
						Subresources: &apiextensionsv1.CustomResourceSubresources{
							Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
						},
						Schema: &apiextensionsv1.CustomResourceValidation{
							OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
								Type:                   "object",
								XPreserveUnknownFields: boolPtr(true),
							},
						},
						AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
							{
								Name:     "Ready",
								Type:     "string",
								JSONPath: ".status.conditions[?(@.type==\"Ready\")].status",
							},
							{
								Name:     "Reason",
								Type:     "string",
								JSONPath: ".status.conditions[?(@.type==\"Ready\")].reason",
							},
						},
					},
				},
			},
		},
	})
}
