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

// UpgradeCustomResourceDefinitionSpecV1Beta1 converts a v1beta1 CustomResourceDefinition to v1.
func UpgradeCustomResourceDefinitionSpecV1Beta1(spec apiextensionsv1beta1.CustomResourceDefinitionSpec) (apiextensionsv1.CustomResourceDefinitionSpec, error) {
	out := apiextensionsv1.CustomResourceDefinitionSpec{
		Group: spec.Group,
		Scope: apiextensionsv1.ResourceScope(spec.Scope),
		Names: apiextensionsv1.CustomResourceDefinitionNames{
			Plural:     spec.Names.Plural,
			Singular:   spec.Names.Singular,
			ShortNames: spec.Names.ShortNames,
			Kind:       spec.Names.Kind,
			ListKind:   spec.Names.ListKind,
			Categories: spec.Names.Categories,
		},
	}
	if spec.Version != "" && len(spec.Versions) == 0 {
		return apiextensionsv1.CustomResourceDefinitionSpec{}, errors.NotValidf("custom resource definition group %q", spec.Group)
	}
	if spec.Versions != nil {
		for _, v := range spec.Versions {
			crd := apiextensionsv1.CustomResourceDefinitionVersion{
				Name:               v.Name,
				Served:             v.Served,
				Storage:            v.Storage,
				Deprecated:         v.Deprecated,
				DeprecationWarning: v.DeprecationWarning,
			}
			if v.Schema != nil {
				schemaBytes, err := json.Marshal(v.Schema)
				if err != nil {
					return apiextensionsv1.CustomResourceDefinitionSpec{}, errors.Trace(err)
				}
				schema := apiextensionsv1.CustomResourceValidation{}
				err = json.Unmarshal(schemaBytes, &schema)
				if err != nil {
					return apiextensionsv1.CustomResourceDefinitionSpec{}, errors.Trace(err)
				}
				crd.Schema = &schema
			}
			if v.Subresources != nil {
				subresourceBytes, err := json.Marshal(v.Subresources)
				if err != nil {
					return apiextensionsv1.CustomResourceDefinitionSpec{}, errors.Trace(err)
				}
				subresource := apiextensionsv1.CustomResourceSubresources{}
				err = json.Unmarshal(subresourceBytes, &subresource)
				if err != nil {
					return apiextensionsv1.CustomResourceDefinitionSpec{}, errors.Trace(err)
				}
				crd.Subresources = &subresource
			}
			if len(v.AdditionalPrinterColumns) > 0 {
				apcBytes, err := json.Marshal(v.AdditionalPrinterColumns)
				if err != nil {
					return apiextensionsv1.CustomResourceDefinitionSpec{}, errors.Trace(err)
				}
				var apc []apiextensionsv1.CustomResourceColumnDefinition
				err = json.Unmarshal(apcBytes, &apc)
				if err != nil {
					return apiextensionsv1.CustomResourceDefinitionSpec{}, errors.Trace(err)
				}
				crd.AdditionalPrinterColumns = apc
			}
			out.Versions = append(out.Versions, crd)
		}
	}
	if spec.PreserveUnknownFields != nil {
		out.PreserveUnknownFields = *spec.PreserveUnknownFields
	}
	if spec.Conversion != nil {
		conversion := apiextensionsv1.CustomResourceConversion{
			Strategy: apiextensionsv1.ConversionStrategyType(spec.Conversion.Strategy),
		}
		if spec.Conversion.WebhookClientConfig != nil {
			conversion.Webhook = &apiextensionsv1.WebhookConversion{
				ConversionReviewVersions: spec.Conversion.ConversionReviewVersions,
				ClientConfig: &apiextensionsv1.WebhookClientConfig{
					URL:      spec.Conversion.WebhookClientConfig.URL,
					CABundle: spec.Conversion.WebhookClientConfig.CABundle,
				},
			}
			if spec.Conversion.WebhookClientConfig.Service != nil {
				conversion.Webhook.ClientConfig.Service = &apiextensionsv1.ServiceReference{
					Namespace: spec.Conversion.WebhookClientConfig.Service.Namespace,
					Name:      spec.Conversion.WebhookClientConfig.Service.Name,
					Path:      spec.Conversion.WebhookClientConfig.Service.Path,
					Port:      spec.Conversion.WebhookClientConfig.Service.Port,
				}
			}
		}
		out.Conversion = &conversion
	}
	if spec.Validation != nil {
		schemaBytes, err := json.Marshal(spec.Validation)
		if err != nil {
			return apiextensionsv1.CustomResourceDefinitionSpec{}, errors.Trace(err)
		}
		schema := apiextensionsv1.CustomResourceValidation{}
		err = json.Unmarshal(schemaBytes, &schema)
		if err != nil {
			return apiextensionsv1.CustomResourceDefinitionSpec{}, errors.Trace(err)
		}
		for i, ver := range out.Versions {
			if ver.Schema == nil {
				ver.Schema = &schema
			}
			out.Versions[i] = ver
		}
	}
	return out, nil
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
	return nil
}
