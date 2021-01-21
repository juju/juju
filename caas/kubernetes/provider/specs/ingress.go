// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"encoding/json"

	"github.com/juju/errors"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
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
	logger.Criticalf("K8sIngressSpec.UnmarshalJSON err %#v", err)
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
