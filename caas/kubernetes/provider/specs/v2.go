// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/juju/juju/caas/specs"
)

type podSpecV2 struct {
	CaaSSpec      specs.PodSpecV2 `json:",inline" yaml:",inline"`
	K8sSpec       K8sPodSpecV2    `json:",inline" yaml:",inline"`
	k8sContainers `json:",inline" yaml:",inline"`
}

// Validate is defined on ProviderPod.
func (p podSpecV2) Validate() error {
	if err := p.CaaSSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.K8sSpec.Validate(); err != nil {
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
	for i, c := range p.Containers {
		pSpec.Containers[i] = c.ToContainerSpec()
	}
	pSpec.Service = p.CaaSSpec.Service
	pSpec.ConfigMaps = p.CaaSSpec.ConfigMaps
	pSpec.ServiceAccount = p.CaaSSpec.ServiceAccount
	pSpec.ProviderPod = &p.K8sSpec
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

// KubernetesResources is the k8s related resources.
type KubernetesResources struct {
	Pod *PodSpec `json:"pod,omitempty" yaml:"pod,omitempty"`

	Secrets                   []Secret                                                     `json:"secrets" yaml:"secrets"`
	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `json:"customResourceDefinitions,omitempty" yaml:"customResourceDefinitions,omitempty"`
	CustomResources           map[string][]unstructured.Unstructured                       `json:"customResources,omitempty" yaml:"customResources,omitempty"`

	ServiceAccounts []K8sServiceAccountSpec `json:"serviceAccounts,omitempty" yaml:"serviceAccounts,omitempty"`
}

// Validate is defined on ProviderPod.
func (krs *KubernetesResources) Validate() error {
	for k, crd := range krs.CustomResourceDefinitions {
		if crd.Scope != apiextensionsv1beta1.NamespaceScoped {
			return errors.NewNotSupported(nil,
				fmt.Sprintf("custom resource definition %q scope %q is not supported, please use %q scope",
					k, crd.Scope, apiextensionsv1beta1.NamespaceScoped),
			)
		}
	}

	for k, crs := range krs.CustomResources {
		if len(crs) == 0 {
			return errors.NotValidf("empty custom resources %q", k)
		}
	}

	for _, sa := range krs.ServiceAccounts {
		if err := sa.Validate(); err != nil {
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
