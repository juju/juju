// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/testing"
)

type ingressSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ingressSuite{})

func (s *ingressSuite) TestK8sIngressV1Beta1(c *gc.C) {
	specV1Beta1 := `
name: test-ingress
labels:
  foo: bar
annotations:
  nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  rules:
    - http:
        paths:
          - path: /testpath
            backend:
              serviceName: test
              servicePort: 80
`
	var obj k8sspecs.K8sIngress
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(specV1Beta1), len(specV1Beta1)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obj, gc.DeepEquals, k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},
		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1Beta1,
			SpecV1Beta1: networkingv1beta1.IngressSpec{
				Rules: []networkingv1beta1.IngressRule{
					{
						IngressRuleValue: networkingv1beta1.IngressRuleValue{
							HTTP: &networkingv1beta1.HTTPIngressRuleValue{
								Paths: []networkingv1beta1.HTTPIngressPath{
									{
										Path: "/testpath",
										Backend: networkingv1beta1.IngressBackend{
											ServiceName: "test",
											ServicePort: intstr.IntOrString{IntVal: 80},
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

func (s *ingressSuite) TestK8sIngressV1(c *gc.C) {
	specV1Beta1 := `
name: ingress-resource-backend
spec:
  defaultBackend:
    resource:
      apiGroup: k8s.example.com
      kind: StorageBucket
      name: static-assets
  rules:
    - http:
        paths:
          - path: /icons
            pathType: ImplementationSpecific
            backend:
              resource:
                apiGroup: k8s.example.com
                kind: StorageBucket
                name: icon-assets
`
	var obj k8sspecs.K8sIngress
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(specV1Beta1), len(specV1Beta1)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)

	pathType := networkingv1.PathTypeImplementationSpecific
	c.Assert(obj, gc.DeepEquals, k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "ingress-resource-backend",
		},
		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1,
			SpecV1: networkingv1.IngressSpec{
				DefaultBackend: &networkingv1.IngressBackend{
					Resource: &core.TypedLocalObjectReference{
						APIGroup: pointer.StringPtr("k8s.example.com"),
						Kind:     "StorageBucket",
						Name:     "static-assets",
					},
				},
				Rules: []networkingv1.IngressRule{
					{
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path:     "/icons",
										PathType: &pathType,
										Backend: networkingv1.IngressBackend{
											Resource: &core.TypedLocalObjectReference{
												APIGroup: pointer.StringPtr("k8s.example.com"),
												Kind:     "StorageBucket",
												Name:     "icon-assets",
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

func (s *ingressSuite) TestIngressSpecToV1(c *gc.C) {
	specV1Beta1 := `
  name: ingress-resource-v1
  spec:
    backend:
      serviceName: fooServiceDefault
      servicePort: 6666
      resource:
        apiGroup: k8s.example.com
        kind: StorageBucket
        name: static-assets
    tls:
      - secretName: shhhh
        hosts: [foo, bar]
    rules:
      - http:
          paths:
            - path: /icons
              pathType: ImplementationSpecific
              backend:
                serviceName: fooService
                servicePort: 8080
                resource:
                  apiGroup: k8s.example.com
                  kind: StorageBucket
                  name: icon-assets
`
	var obj k8sspecs.K8sIngress
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(specV1Beta1), len(specV1Beta1)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)
	specV1 := k8sspecs.IngressSpecToV1(&obj.Spec.SpecV1Beta1)
	pathType := networkingv1.PathTypeImplementationSpecific
	c.Assert(specV1, jc.DeepEquals, &networkingv1.IngressSpec{
		DefaultBackend: &networkingv1.IngressBackend{
			Service: &networkingv1.IngressServiceBackend{
				Name: "fooServiceDefault",
				Port: networkingv1.ServiceBackendPort{Number: 6666},
			},
			Resource: &core.TypedLocalObjectReference{
				APIGroup: pointer.StringPtr("k8s.example.com"),
				Kind:     "StorageBucket",
				Name:     "static-assets",
			},
		},
		TLS: []networkingv1.IngressTLS{{
			Hosts:      []string{"foo", "bar"},
			SecretName: "shhhh",
		}},
		Rules: []networkingv1.IngressRule{
			{
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/icons",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "fooService",
										Port: networkingv1.ServiceBackendPort{
											Number: 8080,
										},
									},
									Resource: &core.TypedLocalObjectReference{
										APIGroup: pointer.StringPtr("k8s.example.com"),
										Kind:     "StorageBucket",
										Name:     "icon-assets",
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

func (s *ingressSuite) TestIngressSpecFromV1(c *gc.C) {
	specV1 := `
  name: ingress-resource-v1
  spec:
    defaultBackend:
      service:
        name: fooServiceDefault
        port: 
          number: 6666
      resource:
        apiGroup: k8s.example.com
        kind: StorageBucket
        name: static-assets
    tls:
      - secretName: shhhh
        hosts: [foo, bar]
    rules:
      - http:
          paths:
            - path: /icons
              pathType: ImplementationSpecific
              backend:
                service:
                  name: fooService
                  port: 
                    number: 8080
                resource:
                  apiGroup: k8s.example.com
                  kind: StorageBucket
                  name: icon-assets
`
	var obj k8sspecs.K8sIngress
	err := k8sspecs.NewStrictYAMLOrJSONDecoder(strings.NewReader(specV1), len(specV1)).Decode(&obj)
	c.Assert(err, jc.ErrorIsNil)
	specV1Beta1 := k8sspecs.IngressSpecFromV1(&obj.Spec.SpecV1)
	pathType := networkingv1beta1.PathTypeImplementationSpecific
	c.Assert(specV1Beta1, jc.DeepEquals, &networkingv1beta1.IngressSpec{
		Backend: &networkingv1beta1.IngressBackend{
			ServiceName: "fooServiceDefault",
			ServicePort: intstr.FromInt(6666),
			Resource: &core.TypedLocalObjectReference{
				APIGroup: pointer.StringPtr("k8s.example.com"),
				Kind:     "StorageBucket",
				Name:     "static-assets",
			},
		},
		TLS: []networkingv1beta1.IngressTLS{{
			Hosts:      []string{"foo", "bar"},
			SecretName: "shhhh",
		}},
		Rules: []networkingv1beta1.IngressRule{
			{
				IngressRuleValue: networkingv1beta1.IngressRuleValue{
					HTTP: &networkingv1beta1.HTTPIngressRuleValue{
						Paths: []networkingv1beta1.HTTPIngressPath{
							{
								Path:     "/icons",
								PathType: &pathType,
								Backend: networkingv1beta1.IngressBackend{
									ServiceName: "fooService",
									ServicePort: intstr.FromInt(8080),
									Resource: &core.TypedLocalObjectReference{
										APIGroup: pointer.StringPtr("k8s.example.com"),
										Kind:     "StorageBucket",
										Name:     "icon-assets",
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
