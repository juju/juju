// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"encoding/base64"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/testing"
)

type webhooksSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&webhooksSuite{})

func (s *webhooksSuite) TestK8sMutatingWebhookV1(c *gc.C) {
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

	webhookFailurePolicy1 := admissionregistrationv1.Ignore
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
				Version: k8sspecs.K8sWebhookV1,
				SpecV1: admissionregistrationv1.MutatingWebhook{
					Name:          "example.mutatingwebhookconfiguration.com",
					FailurePolicy: &webhookFailurePolicy1,
					ClientConfig: admissionregistrationv1.WebhookClientConfig{
						Service: &admissionregistrationv1.ServiceReference{
							Name:      "apple-service",
							Namespace: "apples",
							Path:      strPtr("/apple"),
						},
						CABundle: CABundle,
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{Key: "production", Operator: metav1.LabelSelectorOpDoesNotExist},
						},
					},
					Rules: []admissionregistrationv1.RuleWithOperations{
						admissionregistrationv1.RuleWithOperations{
							Operations: []admissionregistrationv1.OperationType{
								admissionregistrationv1.Create,
								admissionregistrationv1.Update,
							},
							Rule: admissionregistrationv1.Rule{
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

func (s *webhooksSuite) TestK8sValidatingWebhookV1(c *gc.C) {

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
	scope := admissionregistrationv1.NamespacedScope
	sideEffects := admissionregistrationv1.SideEffectClassNone
	c.Assert(obj, gc.DeepEquals, k8sspecs.K8sValidatingWebhook{
		Meta: k8sspecs.Meta{
			Name:        "pod-policy.example.com",
			Labels:      map[string]string{"foo": "bar"},
			Annotations: map[string]string{"juju.io/disable-name-prefix": "true"},
		},
		Webhooks: []k8sspecs.K8sValidatingWebhookSpec{
			{
				Version: k8sspecs.K8sWebhookV1,
				SpecV1: admissionregistrationv1.ValidatingWebhook{
					Name: "pod-policy.example.com",
					Rules: []admissionregistrationv1.RuleWithOperations{
						admissionregistrationv1.RuleWithOperations{
							Operations: []admissionregistrationv1.OperationType{
								admissionregistrationv1.Create,
							},
							Rule: admissionregistrationv1.Rule{
								APIGroups:   []string{""},
								APIVersions: []string{"v1"},
								Resources:   []string{"pods"},
								Scope:       &scope,
							},
						},
					},
					ClientConfig: admissionregistrationv1.WebhookClientConfig{
						Service: &admissionregistrationv1.ServiceReference{
							Name:      "example-service",
							Namespace: "example-namespace",
						},
						CABundle: CABundle,
					},
					AdmissionReviewVersions: []string{"v1", "v1beta1"},
					SideEffects:             &sideEffects,
					TimeoutSeconds:          int32Ptr(5),
				},
			},
		},
	})
}
