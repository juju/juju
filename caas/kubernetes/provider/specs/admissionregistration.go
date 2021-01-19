// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
)

const (
	// K8sWebhookV1Beta1 defines the v1beta1 API version for webhook resources.
	K8sWebhookV1Beta1 APIVersion = "v1beta1"

	// K8sWebhookV1 defines the v1 API version for webhook resources.
	K8sWebhookV1 APIVersion = "v1"
)

// K8sMutatingWebhookSpec defines the spec details of MutatingWebhook with the API version.
type K8sMutatingWebhookSpec struct {
	Version     APIVersion
	SpecV1Beta1 admissionregistrationv1beta1.MutatingWebhook
	SpecV1      admissionregistrationv1.MutatingWebhook
}

// UnmarshalJSON implements the json.Unmarshaller interface.
// NOTE: try v1beta1 first then v1 because admissionregistrationv1
// and admissionregistrationv1beta1 have the same struct but some
// fields might have different required values. To avoid breaking
// existing workloads, we will consider to switch v1 as higher priority in 2.9 instead.
func (wh *K8sMutatingWebhookSpec) UnmarshalJSON(value []byte) (err error) {
	err = unmarshalJSONStrict(value, &wh.SpecV1Beta1)
	if err == nil {
		wh.Version = K8sWebhookV1Beta1
		return nil
	}
	if err2 := unmarshalJSONStrict(value, &wh.SpecV1); err2 == nil {
		wh.Version = K8sWebhookV1
		return nil
	}
	return errors.Trace(err)
}

// MarshalJSON implements the json.Marshaller interface.
func (wh K8sMutatingWebhookSpec) MarshalJSON() ([]byte, error) {
	switch wh.Version {
	case K8sWebhookV1Beta1:
		return json.Marshal(wh.SpecV1Beta1)
	case K8sWebhookV1:
		return json.Marshal(wh.SpecV1)
	default:
		return []byte{}, errors.NotSupportedf("mutating webhook version %q", wh.Version)
	}
}

// K8sValidatingWebhookSpec defines the spec details of ValidatingWebhook with the API version.
type K8sValidatingWebhookSpec struct {
	Version     APIVersion
	SpecV1Beta1 admissionregistrationv1beta1.ValidatingWebhook
	SpecV1      admissionregistrationv1.ValidatingWebhook
}

// UnmarshalJSON implements the json.Unmarshaller interface.
// NOTE: try v1beta1 first then v1 because admissionregistrationv1
// and admissionregistrationv1beta1 have the same struct but some
// fields might have different required values. To avoid breaking
// existing workloads, we will consider to switch v1 as higher priority in 2.9 instead.
func (wh *K8sValidatingWebhookSpec) UnmarshalJSON(value []byte) (err error) {
	err = unmarshalJSONStrict(value, &wh.SpecV1Beta1)
	if err == nil {
		wh.Version = K8sWebhookV1Beta1
		return nil
	}
	if err2 := unmarshalJSONStrict(value, &wh.SpecV1); err2 == nil {
		wh.Version = K8sWebhookV1
		return nil
	}
	return errors.Trace(err)
}

// MarshalJSON implements the json.Marshaller interface.
func (wh K8sValidatingWebhookSpec) MarshalJSON() ([]byte, error) {
	switch wh.Version {
	case K8sWebhookV1Beta1:
		return json.Marshal(wh.SpecV1Beta1)
	case K8sWebhookV1:
		return json.Marshal(wh.SpecV1)
	default:
		return []byte{}, errors.NotSupportedf("validating webhook version %q", wh.Version)
	}
}

func mutatingWebhookFromV1Beta1(whs []admissionregistrationv1beta1.MutatingWebhook) (o []K8sMutatingWebhookSpec) {
	for _, wh := range whs {
		o = append(o, K8sMutatingWebhookSpec{
			Version:     K8sWebhookV1Beta1,
			SpecV1Beta1: wh,
		})
	}
	return o
}

func validatingWebhookFromV1Beta1(whs []admissionregistrationv1beta1.ValidatingWebhook) (o []K8sValidatingWebhookSpec) {
	for _, wh := range whs {
		o = append(o, K8sValidatingWebhookSpec{
			Version:     K8sWebhookV1Beta1,
			SpecV1Beta1: wh,
		})
	}
	return o
}

// K8sMutatingWebhook defines spec for creating or updating an MutatingWebhook resource.
type K8sMutatingWebhook struct {
	Meta     `json:",inline" yaml:",inline"`
	Webhooks []K8sMutatingWebhookSpec `json:"webhooks" yaml:"webhooks"`
}

// APIVersion returns the API version.
func (w *K8sMutatingWebhook) APIVersion() APIVersion {
	return w.Webhooks[0].Version
}

// Validate validates the spec.
func (w K8sMutatingWebhook) Validate() error {
	if err := w.Meta.Validate(); err != nil {
		return errors.Trace(err)
	}
	if len(w.Webhooks) == 0 {
		return errors.NotValidf("empty webhooks %q", w.Name)
	}
	ver := w.APIVersion()
	for _, v := range w.Webhooks[1:] {
		if v.Version != ver {
			return errors.NewNotValid(nil, fmt.Sprintf("more than one version of webhooks in same spec, found %q and %q", ver, v.Version))
		}
	}
	return nil
}

// K8sValidatingWebhook defines spec for creating or updating an ValidatingWebhook resource.
type K8sValidatingWebhook struct {
	Meta     `json:",inline" yaml:",inline"`
	Webhooks []K8sValidatingWebhookSpec `json:"webhooks" yaml:"webhooks"`
}

// APIVersion returns the API version.
func (w *K8sValidatingWebhook) APIVersion() APIVersion {
	return w.Webhooks[0].Version
}

// Validate validates the spec.
func (w *K8sValidatingWebhook) Validate() error {
	if err := w.Meta.Validate(); err != nil {
		return errors.Trace(err)
	}
	if len(w.Webhooks) == 0 {
		return errors.NotValidf("empty webhooks %q", w.Name)
	}
	ver := w.APIVersion()
	for _, v := range w.Webhooks[1:] {
		if v.Version != ver {
			return errors.NewNotValid(nil, fmt.Sprintf("more than one version of webhooks in same spec, found %q and %q", ver, v.Version))
		}
	}
	return nil
}
