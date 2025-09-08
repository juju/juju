// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"encoding/base64"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/testing"
)

type webhooksSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&webhooksSuite{})

func (s *webhooksSuite) TestK8sMutatingWebhookV1Beta1(c *gc.C) {
	specV1Beta1 := `
name: example-mutatingwebhookconfiguration
labels:
  foo: bar
annotations:
  juju.io/disable-name-prefix: "true"
webhooks:
  - name: "example.mutatingwebhookconfiguration.com"
    failurePolicy: Ignore
    clientConfig:
      service:
        name: apple-service
        namespace: apples
        path: /apple
      caBundle: "YXBwbGVz"
    namespaceSelector:
      matchExpressions:
        - key: production
          operator: DoesNotExist
    rules:
      - apiGroups:
          - ""
        apiVersions:
          - v1
        operations:
          - CREATE
          - UPDATE
        resources:
          - pods
`
	var obj k8sspecs.K8sMutatingWebhook
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(specV1Beta1), len(specV1Beta1)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)

	webhookFailurePolicy1 := admissionregistrationv1beta1.Ignore
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obj, gc.DeepEquals, k8sspecs.K8sMutatingWebhook{
		Meta: k8sspecs.Meta{
			Name:        "example-mutatingwebhookconfiguration",
			Labels:      map[string]string{"foo": "bar"},
			Annotations: map[string]string{"juju.io/disable-name-prefix": "true"},
		},
		Webhooks: []k8sspecs.K8sMutatingWebhookSpec{
			{
				Version: k8sspecs.K8sWebhookV1Beta1,
				SpecV1Beta1: admissionregistrationv1beta1.MutatingWebhook{
					Name:          "example.mutatingwebhookconfiguration.com",
					FailurePolicy: &webhookFailurePolicy1,
					ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
						Service: &admissionregistrationv1beta1.ServiceReference{
							Name:      "apple-service",
							Namespace: "apples",
							Path:      pointer.StringPtr("/apple"),
						},
						CABundle: CABundle,
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
						},
					},
					Rules: []admissionregistrationv1beta1.RuleWithOperations{
						{
							Operations: []admissionregistrationv1beta1.OperationType{
								admissionregistrationv1beta1.Create,
								admissionregistrationv1beta1.Update,
							},
							Rule: admissionregistrationv1beta1.Rule{
								APIGroups:   []string{""},
								APIVersions: []string{"v1"},
								Resources:   []string{"pods"},
							},
						},
					},
				},
			},
		},
	})
}

func (s *webhooksSuite) TestK8sMutatingWebhookV1(c *gc.C) {
	specV1 := `
name: example-mutatingwebhookconfiguration
labels:
  foo: bar
annotations:
  juju.io/disable-name-prefix: "true"
webhooks:
  - name: "example.mutatingwebhookconfiguration.com"
    failurePolicy: Ignore
    clientConfig:
      service:
        name: apple-service
        namespace: apples
        path: /apple
      caBundle: "YXBwbGVz"
    namespaceSelector:
      matchExpressions:
        - key: production
          operator: DoesNotExist
    rules:
      - apiGroups:
          - ""
        apiVersions:
          - v1
        operations:
          - CREATE
          - UPDATE
        resources:
          - pods
`
	var obj k8sspecs.K8sMutatingWebhook
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(specV1), len(specV1)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)

	webhookFailurePolicy1 := admissionregistrationv1beta1.Ignore
	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obj, gc.DeepEquals, k8sspecs.K8sMutatingWebhook{
		Meta: k8sspecs.Meta{
			Name:        "example-mutatingwebhookconfiguration",
			Labels:      map[string]string{"foo": "bar"},
			Annotations: map[string]string{"juju.io/disable-name-prefix": "true"},
		},
		Webhooks: []k8sspecs.K8sMutatingWebhookSpec{
			{
				Version: k8sspecs.K8sWebhookV1Beta1,
				SpecV1Beta1: admissionregistrationv1beta1.MutatingWebhook{
					Name:          "example.mutatingwebhookconfiguration.com",
					FailurePolicy: &webhookFailurePolicy1,
					ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
						Service: &admissionregistrationv1beta1.ServiceReference{
							Name:      "apple-service",
							Namespace: "apples",
							Path:      pointer.StringPtr("/apple"),
						},
						CABundle: CABundle,
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
						},
					},
					Rules: []admissionregistrationv1beta1.RuleWithOperations{
						{
							Operations: []admissionregistrationv1beta1.OperationType{
								admissionregistrationv1beta1.Create,
								admissionregistrationv1beta1.Update,
							},
							Rule: admissionregistrationv1beta1.Rule{
								APIGroups:   []string{""},
								APIVersions: []string{"v1"},
								Resources:   []string{"pods"},
							},
						},
					},
				},
			},
		},
	})
}

func (s *webhooksSuite) TestK8sMutatingWebhookInvalid(c *gc.C) {
	spec := `
name: example-mutatingwebhookconfiguration
labels:
  foo: bar
annotations:
  juju.io/disable-name-prefix: "true"
`
	var obj k8sspecs.K8sMutatingWebhook
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(spec), len(spec)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obj.Validate(), gc.ErrorMatches, `empty webhooks "example-mutatingwebhookconfiguration" not valid`)
}

func (s *webhooksSuite) TestK8sValidatingWebhookV1Beta1(c *gc.C) {

	specV1Beta1 := `
name: pod-policy.example.com
labels:
  foo: bar
annotations:
  juju.io/disable-name-prefix: "true"
webhooks:
  - name: "pod-policy.example.com"
    rules:
      - apiGroups: [""]
        apiVersions: ["v1"]
        operations: ["CREATE"]
        resources: ["pods"]
        scope: "Namespaced"
    clientConfig:
      service:
        namespace: "example-namespace"
        name: "example-service"
      caBundle: "YXBwbGVz"
    admissionReviewVersions: ["v1", "v1beta1"]
    sideEffects: None
    timeoutSeconds: 5
`
	var obj k8sspecs.K8sValidatingWebhook
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(specV1Beta1), len(specV1Beta1)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)

	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	scope := admissionregistrationv1beta1.NamespacedScope
	sideEffects := admissionregistrationv1beta1.SideEffectClassNone
	c.Assert(obj, gc.DeepEquals, k8sspecs.K8sValidatingWebhook{
		Meta: k8sspecs.Meta{
			Name:        "pod-policy.example.com",
			Labels:      map[string]string{"foo": "bar"},
			Annotations: map[string]string{"juju.io/disable-name-prefix": "true"},
		},
		Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
			{
				Version: k8sspecs.K8sWebhookV1Beta1,
				SpecV1Beta1: admissionregistrationv1beta1.ValidatingWebhook{
					Name: "pod-policy.example.com",
					Rules: []admissionregistrationv1beta1.RuleWithOperations{
						{
							Operations: []admissionregistrationv1beta1.OperationType{
								admissionregistrationv1beta1.Create,
							},
							Rule: admissionregistrationv1beta1.Rule{
								APIGroups:   []string{""},
								APIVersions: []string{"v1"},
								Resources:   []string{"pods"},
								Scope:       &scope,
							},
						},
					},
					ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
						Service: &admissionregistrationv1beta1.ServiceReference{
							Name:      "example-service",
							Namespace: "example-namespace",
						},
						CABundle: CABundle,
					},
					AdmissionReviewVersions: []string{"v1", "v1beta1"},
					SideEffects:             &sideEffects,
					TimeoutSeconds:          pointer.Int32Ptr(5),
				},
			},
		},
	})
}

func (s *webhooksSuite) TestK8sValidatingWebhookV1(c *gc.C) {

	specV1 := `
name: pod-policy.example.com
labels:
  foo: bar
annotations:
  juju.io/disable-name-prefix: "true"
webhooks:
  - name: "pod-policy.example.com"
    rules:
      - apiGroups: [""]
        apiVersions: ["v1"]
        operations: ["CREATE"]
        resources: ["pods"]
        scope: "Namespaced"
    clientConfig:
      service:
        namespace: "example-namespace"
        name: "example-service"
      caBundle: "YXBwbGVz"
    admissionReviewVersions: ["v1", "v1beta1"]
    sideEffects: None
    timeoutSeconds: 5
`
	var obj k8sspecs.K8sValidatingWebhook
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(specV1), len(specV1)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)

	CABundle, err := base64.StdEncoding.DecodeString("YXBwbGVz")
	c.Assert(err, jc.ErrorIsNil)
	scope := admissionregistrationv1beta1.NamespacedScope
	sideEffects := admissionregistrationv1beta1.SideEffectClassNone
	c.Assert(obj, gc.DeepEquals, k8sspecs.K8sValidatingWebhook{
		Meta: k8sspecs.Meta{
			Name:        "pod-policy.example.com",
			Labels:      map[string]string{"foo": "bar"},
			Annotations: map[string]string{"juju.io/disable-name-prefix": "true"},
		},
		Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
			{
				Version: k8sspecs.K8sWebhookV1Beta1,
				SpecV1Beta1: admissionregistrationv1beta1.ValidatingWebhook{
					Name: "pod-policy.example.com",
					Rules: []admissionregistrationv1beta1.RuleWithOperations{
						{
							Operations: []admissionregistrationv1beta1.OperationType{
								admissionregistrationv1beta1.Create,
							},
							Rule: admissionregistrationv1beta1.Rule{
								APIGroups:   []string{""},
								APIVersions: []string{"v1"},
								Resources:   []string{"pods"},
								Scope:       &scope,
							},
						},
					},
					ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
						Service: &admissionregistrationv1beta1.ServiceReference{
							Name:      "example-service",
							Namespace: "example-namespace",
						},
						CABundle: CABundle,
					},
					AdmissionReviewVersions: []string{"v1", "v1beta1"},
					SideEffects:             &sideEffects,
					TimeoutSeconds:          pointer.Int32Ptr(5),
				},
			},
		},
	})
}

func (s *webhooksSuite) TestK8sValidatingWebhookInvalid(c *gc.C) {

	spec := `
name: pod-policy.example.com
labels:
  foo: bar
annotations:
  juju.io/disable-name-prefix: "true"
`
	var obj k8sspecs.K8sValidatingWebhook
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(spec), len(spec)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obj.Validate(), gc.ErrorMatches, `empty webhooks "pod-policy.example.com" not valid`)
}
