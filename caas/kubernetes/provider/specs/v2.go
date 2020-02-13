// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/juju/juju/caas/specs"
)

type caaSSpecV2 = specs.PodSpecV2

type podSpecV2 struct {
	caaSSpecV2    `json:",inline" yaml:",inline"`
	K8sPodSpecV2  `json:",inline" yaml:",inline"`
	k8sContainers `json:",inline" yaml:",inline"`
}

// Validate is defined on ProviderPod.
func (p podSpecV2) Validate() error {
	if err := p.K8sPodSpecV2.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.k8sContainers.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p podSpecV2) ToLatest() *specs.PodSpec {
	pSpec := &specs.PodSpec{}
	pSpec.Version = specs.CurrentVersion
	// TOD(caas): OmitServiceFrontend is deprecated in v2 and will be removed in v3.
	pSpec.OmitServiceFrontend = false
	for _, c := range p.Containers {
		pSpec.Containers = append(pSpec.Containers, c.ToContainerSpec())
	}
	pSpec.Service = p.caaSSpecV2.Service
	pSpec.ConfigMaps = p.caaSSpecV2.ConfigMaps
	pSpec.ServiceAccount = p.caaSSpecV2.ServiceAccount
	pSpec.ProviderPod = &p.K8sPodSpecV2
	return pSpec
}

// K8sPodSpecV2 is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type K8sPodSpecV2 struct {
	// k8s resources.
	KubernetesResources *KubernetesResources `json:"kubernetesResources,omitempty" yaml:"kubernetesResources,omitempty"`
}

// Validate is defined on ProviderPod.
func (p *K8sPodSpecV2) Validate() error {
	if p.KubernetesResources != nil {
		if err := p.KubernetesResources.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// K8sServiceAccountSpec defines spec for referencing or creating a service account.
type K8sServiceAccountSpec struct {
	Name           string `json:"name" yaml:"name"`
	specs.RBACSpec `json:",inline" yaml:",inline"`
}

// GetName returns the service accout name.
func (sa K8sServiceAccountSpec) GetName() string {
	return sa.Name
}

// GetSpec returns the RBAC spec.
func (sa K8sServiceAccountSpec) GetSpec() specs.RBACSpec {
	return sa.RBACSpec
}

// Validate returns an error if the spec is not valid.
func (sa K8sServiceAccountSpec) Validate() error {
	if sa.Name == "" {
		return errors.New("service account name is missing")
	}
	return errors.Trace(sa.RBACSpec.Validate())
}

// K8sIngressSpec defines spec for creating or updating an ingress resource.
type K8sIngressSpec struct {
	Name        string                        `json:"name" yaml:"name"`
	Labels      map[string]string             `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string             `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	Spec        extensionsv1beta1.IngressSpec `json:"spec" yaml:"spec"`
}

// Validate returns an error if the spec is not valid.
func (ing K8sIngressSpec) Validate() error {
	if ing.Name == "" {
		return errors.New("ingress name is missing")
	}
	return nil
}

// KubernetesResources is the k8s related resources.
type KubernetesResources struct {
	Pod *PodSpec `json:"pod,omitempty" yaml:"pod,omitempty"`

	Secrets                   []Secret                                                     `json:"secrets" yaml:"secrets"`
	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `json:"customResourceDefinitions,omitempty" yaml:"customResourceDefinitions,omitempty"`
	CustomResources           map[string][]unstructured.Unstructured                       `json:"customResources,omitempty" yaml:"customResources,omitempty"`

	MutatingWebhookConfigurations   map[string][]admissionregistration.MutatingWebhook   `json:"mutatingWebhookConfigurations,omitempty" yaml:"mutatingWebhookConfigurations,omitempty"`
	ValidatingWebhookConfigurations map[string][]admissionregistration.ValidatingWebhook `json:"validatingWebhookConfigurations,omitempty" yaml:"validatingWebhookConfigurations,omitempty"`

	ServiceAccounts  []K8sServiceAccountSpec `json:"serviceAccounts,omitempty" yaml:"serviceAccounts,omitempty"`
	IngressResources []K8sIngressSpec        `json:"ingressResources,omitempty" yaml:"ingressResources,omitempty"`
}

func validateCustomResourceDefinition(name string, crd apiextensionsv1beta1.CustomResourceDefinitionSpec) error {
	if crd.Scope != apiextensionsv1beta1.NamespaceScoped && crd.Scope != apiextensionsv1beta1.ClusterScoped {
		return errors.NewNotSupported(nil,
			fmt.Sprintf("custom resource definition %q scope %q is not supported, please use %q or %q scope",
				name, crd.Scope, apiextensionsv1beta1.NamespaceScoped, apiextensionsv1beta1.ClusterScoped),
		)
	}
	return nil
}

// Validate is defined on ProviderPod.
func (krs *KubernetesResources) Validate() error {
	for k, crd := range krs.CustomResourceDefinitions {
		if err := validateCustomResourceDefinition(k, crd); err != nil {
			return errors.Trace(err)
		}
	}

	for k, crs := range krs.CustomResources {
		if len(crs) == 0 {
			return errors.NotValidf("empty custom resources %q", k)
		}
	}
	for k, webhooks := range krs.MutatingWebhookConfigurations {
		if len(webhooks) == 0 {
			return errors.NotValidf("empty webhooks %q", k)
		}
	}
	for k, webhooks := range krs.ValidatingWebhookConfigurations {
		if len(webhooks) == 0 {
			return errors.NotValidf("empty webhooks %q", k)
		}
	}

	for _, sa := range krs.ServiceAccounts {
		if err := sa.Validate(); err != nil {
			return errors.Trace(err)
		}
	}

	for _, ing := range krs.IngressResources {
		if err := ing.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func parsePodSpecV2(in string) (_ PodSpecConverter, err error) {
	var spec podSpecV2
	decoder := newStrictYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err = decoder.Decode(&spec); err != nil {
		return nil, errors.Trace(err)
	}
	return &spec, nil
}
