// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/juju/juju/caas/specs"
)

type caaSSpecV3 = specs.PodSpecV3

type podSpecV3 struct {
	caaSSpecV3    `json:",inline" yaml:",inline"`
	K8sPodSpecV3  `json:",inline" yaml:",inline"`
	k8sContainers `json:",inline" yaml:",inline"`
}

// Validate is defined on ProviderPod.
func (p podSpecV3) Validate() error {
	if err := p.K8sPodSpecV3.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.k8sContainers.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p podSpecV3) ToLatest() *specs.PodSpec {
	pSpec := &specs.PodSpec{}
	pSpec.Version = specs.CurrentVersion
	for _, c := range p.Containers {
		pSpec.Containers = append(pSpec.Containers, c.ToContainerSpec())
	}
	pSpec.Service = p.caaSSpecV3.Service
	pSpec.ConfigMaps = p.caaSSpecV3.ConfigMaps
	pSpec.ServiceAccount = p.caaSSpecV3.ServiceAccount
	pSpec.ProviderPod = &p.K8sPodSpecV3
	return pSpec
}

// RoleSpec defines a spec for creating a role or cluster role.
type RoleSpec struct {
	Name   string             `json:"name" yaml:"name"`
	Global bool               `json:"global" yaml:"global"`
	Rules  []specs.PolicyRule `json:"rules" yaml:"rules"`
}

// Validate returns an error if the spec is not valid.
func (rs RoleSpec) Validate() error {
	if rs.Name == "" {
		return errors.New("spec name is missing")
	}
	if len(rs.Rules) == 0 {
		return errors.NewNotValid(nil, "rules is required")
	}
	return nil
}

// K8sServiceAccountSpec defines spec for referencing or creating additional RBAC resources.
type K8sServiceAccountSpec struct {
	Name string `json:"name" yaml:"name"`
	// RBACSpec `yaml:",inline"`
	AutomountServiceAccountToken *bool    `yaml:"automountServiceAccountToken,omitempty"`
	Roles                        []string `json:"roles" yaml:"roles"`
}

// Validate returns an error if the spec is not valid.
func (sa K8sServiceAccountSpec) Validate() error {
	if sa.Name == "" {
		return errors.New("name is missing")
	}
	return nil
	// return errors.Trace(sa.RBACSpec.Validate())
}

// K8sRBACResources defines a spec for creating RBAC resources.
type K8sRBACResources struct {
	K8sSpecConverter
	ServiceAccounts     []K8sServiceAccountSpec `json:"serviceAccounts,omitempty" yaml:"serviceAccounts,omitempty"`
	ServiceAccountRoles []RoleSpec              `json:"serviceAccountRoles,omitempty" yaml:"serviceAccountRoles,omitempty"`
}

// K8sSpecConverter has a method to convert modelled spec to k8s spec.
type K8sSpecConverter interface {
	ToK8s()
}

// Validate is defined on ProviderPod.
func (ks K8sRBACResources) Validate() error {
	roleNames := set.NewStrings()
	for _, r := range ks.ServiceAccountRoles {
		if err := r.Validate(); err != nil {
			return errors.Trace(err)
		}
		if roleNames.Contains(r.Name) {
			return errors.NotValidf("duplicated role name %q", r.Name)
		}
		roleNames.Add(r.Name)
	}
	for _, sa := range ks.ServiceAccounts {
		if err := sa.Validate(); err != nil {
			return errors.Trace(err)
		}
		for _, rName := range sa.Roles {
			if !roleNames.Contains(rName) {
				return errors.NewNotValid(nil, fmt.Sprintf("service account %q references an unknown role %q", sa.Name, rName))
			}
		}
	}
	return nil
}

// ToK8s converts modelled spec to k8s spec.
func (ks K8sRBACResources) ToK8s() ([]core.ServiceAccount, []rbacv1.Role, []rbacv1.ClusterRole) {
	// TODO:!!
	return nil, nil, nil
}

// KubernetesResources is the k8s related resources.
type KubernetesResources struct {
	Pod *PodSpec `json:"pod,omitempty" yaml:"pod,omitempty"`

	Secrets                   []Secret                                                     `json:"secrets" yaml:"secrets"`
	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `json:"customResourceDefinitions,omitempty" yaml:"customResourceDefinitions,omitempty"`
	CustomResources           map[string][]unstructured.Unstructured                       `json:"customResources,omitempty" yaml:"customResources,omitempty"`

	MutatingWebhookConfigurations   map[string][]admissionregistration.MutatingWebhook   `json:"mutatingWebhookConfigurations,omitempty" yaml:"mutatingWebhookConfigurations,omitempty"`
	ValidatingWebhookConfigurations map[string][]admissionregistration.ValidatingWebhook `json:"validatingWebhookConfigurations,omitempty" yaml:"validatingWebhookConfigurations,omitempty"`

	K8sRBACResources `json:",inline" yaml:",inline"`

	IngressResources []K8sIngressSpec `json:"ingressResources,omitempty" yaml:"ingressResources,omitempty"`
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

	if err := krs.K8sRBACResources.Validate(); err != nil {
		return errors.Trace(err)
	}

	for _, ing := range krs.IngressResources {
		if err := ing.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// K8sPodSpecV3 is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type K8sPodSpecV3 struct {
	// k8s resources.
	KubernetesResources *KubernetesResources `json:"kubernetesResources,omitempty" yaml:"kubernetesResources,omitempty"`
}

// Validate is defined on ProviderPod.
func (p *K8sPodSpecV3) Validate() error {
	if p.KubernetesResources != nil {
		if err := p.KubernetesResources.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func parsePodSpecV3(in string) (_ PodSpecConverter, err error) {
	var spec podSpecV3
	decoder := newStrictYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err = decoder.Decode(&spec); err != nil {
		return nil, errors.Trace(err)
	}
	return &spec, nil
}
