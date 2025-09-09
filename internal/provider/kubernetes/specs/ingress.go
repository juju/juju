// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"encoding/json"

	"github.com/juju/errors"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// K8sIngressV1Beta1 defines the v1beta1 API version for ingress.
	K8sIngressV1Beta1 APIVersion = "v1beta1"

	// K8sIngressV1 defines the v1 API version for ingress.
	K8sIngressV1 APIVersion = "v1"
)

// K8sIngressSpec defines the spec details of the Ingress with the API version.
type K8sIngressSpec struct {
	Version     APIVersion
	SpecV1Beta1 networkingv1beta1.IngressSpec
	SpecV1      networkingv1.IngressSpec
}

// UnmarshalJSON implements the json.Unmarshaller interface.
func (ing *K8sIngressSpec) UnmarshalJSON(value []byte) (err error) {
	err = unmarshalJSONStrict(value, &ing.SpecV1)
	if err == nil {
		ing.Version = K8sIngressV1
		return nil
	}
	if err2 := unmarshalJSONStrict(value, &ing.SpecV1Beta1); err2 == nil {
		ing.Version = K8sIngressV1Beta1
		return nil
	}
	return errors.Trace(err)
}

// MarshalJSON implements the json.Marshaller interface.
func (ing K8sIngressSpec) MarshalJSON() ([]byte, error) {
	switch ing.Version {
	case K8sIngressV1Beta1:
		return json.Marshal(ing.SpecV1Beta1)
	case K8sIngressV1:
		return json.Marshal(ing.SpecV1)
	default:
		return []byte{}, errors.NotSupportedf("ingress version %q", ing.Version)
	}
}

// K8sIngress defines spec for creating or updating an ingress resource.
type K8sIngress struct {
	Meta `json:",inline" yaml:",inline"`
	Spec K8sIngressSpec `json:"spec" yaml:"spec"`
}

// Validate returns an error if the spec is not valid.
func (ing K8sIngress) Validate() error {
	if err := ing.Meta.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// IngressSpecToV1 converts a beta1 spec to the equivalent v1 version.
func IngressSpecToV1(in *networkingv1beta1.IngressSpec) *networkingv1.IngressSpec {
	if in == nil {
		return nil
	}
	out := &networkingv1.IngressSpec{
		IngressClassName: in.IngressClassName,
		DefaultBackend:   backendToV1(in.Backend),
	}
	for _, tls := range in.TLS {
		out.TLS = append(out.TLS, networkingv1.IngressTLS{
			Hosts:      tls.Hosts,
			SecretName: tls.SecretName,
		})
	}
	for _, rule := range in.Rules {
		out.Rules = append(out.Rules, networkingv1.IngressRule{
			Host: rule.Host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: httpRuleToV1(rule.HTTP),
			},
		})
	}
	return out
}

func backendToV1(in *networkingv1beta1.IngressBackend) *networkingv1.IngressBackend {
	if in == nil {
		return nil
	}
	out := &networkingv1.IngressBackend{
		Service:  serviceToV1(in),
		Resource: in.Resource,
	}
	return out
}

func serviceToV1(in *networkingv1beta1.IngressBackend) *networkingv1.IngressServiceBackend {
	if in == nil || in.ServiceName == "" {
		return nil
	}
	out := &networkingv1.IngressServiceBackend{
		Name: in.ServiceName,
		Port: networkingv1.ServiceBackendPort{
			Name:   in.ServicePort.StrVal,
			Number: in.ServicePort.IntVal,
		},
	}
	return out
}

func httpRuleToV1(in *networkingv1beta1.HTTPIngressRuleValue) *networkingv1.HTTPIngressRuleValue {
	if in == nil {
		return nil
	}
	out := &networkingv1.HTTPIngressRuleValue{}
	for _, path := range in.Paths {
		outPath := networkingv1.HTTPIngressPath{
			Path:    path.Path,
			Backend: *backendToV1(&path.Backend),
		}
		pathType := networkingv1.PathTypeImplementationSpecific
		if path.PathType != nil {
			pathType = networkingv1.PathType(*path.PathType)
		}
		outPath.PathType = &pathType
		out.Paths = append(out.Paths, outPath)
	}
	return out
}

// IngressSpecFromV1 converts a v1 spec to the equivalent v1beta1 version.
func IngressSpecFromV1(in *networkingv1.IngressSpec) *networkingv1beta1.IngressSpec {
	if in == nil {
		return nil
	}
	out := &networkingv1beta1.IngressSpec{
		IngressClassName: in.IngressClassName,
		Backend:          backendToBeta1(in.DefaultBackend),
	}
	for _, tls := range in.TLS {
		out.TLS = append(out.TLS, networkingv1beta1.IngressTLS{
			Hosts:      tls.Hosts,
			SecretName: tls.SecretName,
		})
	}
	for _, rule := range in.Rules {
		out.Rules = append(out.Rules, networkingv1beta1.IngressRule{
			Host: rule.Host,
			IngressRuleValue: networkingv1beta1.IngressRuleValue{
				HTTP: httpRuleToBeta1(rule.HTTP),
			},
		})
	}
	return out
}

func backendToBeta1(in *networkingv1.IngressBackend) *networkingv1beta1.IngressBackend {
	if in == nil {
		return nil
	}
	out := &networkingv1beta1.IngressBackend{
		Resource: in.Resource,
	}
	if in.Service != nil {
		out.ServiceName = in.Service.Name
		if in.Service.Port.Name != "" {
			out.ServicePort = intstr.FromString(in.Service.Port.Name)
		} else {
			out.ServicePort = intstr.FromInt(int(in.Service.Port.Number))
		}
	}
	return out
}

func httpRuleToBeta1(in *networkingv1.HTTPIngressRuleValue) *networkingv1beta1.HTTPIngressRuleValue {
	if in == nil {
		return nil
	}
	out := &networkingv1beta1.HTTPIngressRuleValue{}
	for _, path := range in.Paths {
		outPath := networkingv1beta1.HTTPIngressPath{
			Path:    path.Path,
			Backend: *backendToBeta1(&path.Backend),
		}
		if path.PathType != nil {
			pathType := networkingv1beta1.PathType(*path.PathType)
			outPath.PathType = &pathType
		}
		out.Paths = append(out.Paths, outPath)
	}
	return out
}
