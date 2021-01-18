// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

const (
	// K8sCustomResourceDefinitionV1Beta1 defines the v1beta1 API version for custom resource definition.
	K8sCustomResourceDefinitionV1Beta1 APIVersion = "v1beta1"

	// K8sCustomResourceDefinitionV1 defines the v1 API version for custom resource definition.
	K8sCustomResourceDefinitionV1 APIVersion = "v1"
)

// K8sCustomResourceDefinitionSpec defines the spec details of CustomResourceDefinition with the API version.
type K8sCustomResourceDefinitionSpec struct {
	Version     APIVersion
	SpecV1Beta1 apiextensionsv1beta1.CustomResourceDefinitionSpec
	SpecV1      apiextensionsv1.CustomResourceDefinitionSpec
}

// UnmarshalJSON implements the json.Unmarshaller interface.
func (crdSpecs *K8sCustomResourceDefinitionSpec) UnmarshalJSON(value []byte) (err error) {
	err = unmarshalJSONStrict(value, &crdSpecs.SpecV1)
	if err == nil {
		crdSpecs.Version = K8sCustomResourceDefinitionV1
		return nil
	}
	if err2 := unmarshalJSONStrict(value, &crdSpecs.SpecV1Beta1); err2 == nil {
		crdSpecs.Version = K8sCustomResourceDefinitionV1Beta1
		return nil
	}
	return errors.Trace(err)
}

// MarshalJSON implements the json.Marshaller interface.
func (crdSpecs K8sCustomResourceDefinitionSpec) MarshalJSON() ([]byte, error) {
	switch crdSpecs.Version {
	case K8sCustomResourceDefinitionV1Beta1:
		return json.Marshal(crdSpecs.SpecV1Beta1)
	case K8sCustomResourceDefinitionV1:
		return json.Marshal(crdSpecs.SpecV1)
	default:
		return []byte{}, errors.NotSupportedf("custom resource definition version %q", crdSpecs.Version)
	}
}

// Validate validates the spec.
func (crdSpecs K8sCustomResourceDefinitionSpec) Validate(name string) error {
	switch crdSpecs.Version {
	case K8sCustomResourceDefinitionV1Beta1:
		if crdSpecs.SpecV1Beta1.Scope != apiextensionsv1beta1.NamespaceScoped && crdSpecs.SpecV1Beta1.Scope != apiextensionsv1beta1.ClusterScoped {
			return errors.NewNotSupported(nil,
				fmt.Sprintf("custom resource definition %q scope %q is not supported, please use %q or %q scope",
					name, crdSpecs.SpecV1Beta1.Scope, apiextensionsv1beta1.NamespaceScoped, apiextensionsv1beta1.ClusterScoped),
			)
		}
	case K8sCustomResourceDefinitionV1:
		if crdSpecs.SpecV1.Scope != apiextensionsv1.NamespaceScoped && crdSpecs.SpecV1.Scope != apiextensionsv1.ClusterScoped {
			return errors.NewNotSupported(nil,
				fmt.Sprintf("custom resource definition %q scope %q is not supported, please use %q or %q scope",
					name, crdSpecs.SpecV1.Scope, apiextensionsv1.NamespaceScoped, apiextensionsv1.ClusterScoped),
			)
		}
	default:
		return errors.NotSupportedf("custom resource definition %q version %q", name, crdSpecs.Version)
	}
	return nil
}

// K8sCustomResourceDefinition defines spec for creating or updating an CustomResourceDefinition resource.
type K8sCustomResourceDefinition struct {
	Meta `json:",inline" yaml:",inline"`
	Spec K8sCustomResourceDefinitionSpec `json:"spec" yaml:"spec"`
}

// Validate validates the spec.
func (crd K8sCustomResourceDefinition) Validate() error {
	if err := crd.Meta.Validate(); err != nil {
		return errors.Trace(err)
	}

	if err := crd.Spec.Validate(crd.Name); err != nil {
		return errors.Trace(err)
	}

	if err := validateLabels(crd.Labels); err != nil {
		return errors.Trace(err)
	}
	return nil
}
