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

type k8sContainerV2 struct {
	specs.ContainerSpecV2 `json:",inline" yaml:",inline"`
	Kubernetes            *K8sContainerSpec `json:"kubernetes,omitempty" yaml:"kubernetes,omitempty"`
}

// Validate validates k8sContainerV2.
func (c *k8sContainerV2) Validate() error {
	if err := c.ContainerSpecV2.Validate(); err != nil {
		return errors.Trace(err)
	}
	if c.Kubernetes != nil {
		if err := c.Kubernetes.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func fileSetsV2ToFileSets(fs []specs.FileSetV2) (out []specs.FileSet) {
	for _, f := range fs {
		newf := specs.FileSet{
			Name:      f.Name,
			MountPath: f.MountPath,
		}
		for k, v := range f.Files {
			newf.Files = append(newf.Files, specs.File{
				Path:    k,
				Content: v,
			})
		}
		out = append(out, newf)
	}
	return out
}

func (c *k8sContainerV2) ToContainerSpec() specs.ContainerSpec {
	result := specs.ContainerSpec{
		ImageDetails:    c.ImageDetails,
		Name:            c.Name,
		Init:            c.Init,
		Image:           c.Image,
		Ports:           c.Ports,
		Command:         c.Command,
		Args:            c.Args,
		WorkingDir:      c.WorkingDir,
		EnvConfig:       c.Config,
		VolumeConfig:    fileSetsV2ToFileSets(c.Files),
		ImagePullPolicy: c.ImagePullPolicy,
	}
	if c.Kubernetes != nil {
		result.ProviderContainer = c.Kubernetes
	}
	return result
}

type k8sContainersV2 struct {
	Containers []k8sContainerV2 `json:"containers" yaml:"containers"`
}

// Validate is defined on ProviderContainer.
func (cs *k8sContainersV2) Validate() error {
	if len(cs.Containers) == 0 {
		return errors.New("require at least one container spec")
	}
	for _, c := range cs.Containers {
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

type caaSSpecV2 = specs.PodSpecV2

type podSpecV2 struct {
	caaSSpecV2      `json:",inline" yaml:",inline"`
	K8sPodSpecV2    `json:",inline" yaml:",inline"`
	k8sContainersV2 `json:",inline" yaml:",inline"`
}

// Validate is defined on ProviderPod.
func (p podSpecV2) Validate() error {
	if err := p.K8sPodSpecV2.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.k8sContainersV2.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p podSpecV2) ToLatest() *specs.PodSpec {
	pSpec := &specs.PodSpec{}
	pSpec.Version = specs.CurrentVersion
	// TODO(caas): OmitServiceFrontend is deprecated in v2 and will be removed in a later version.
	pSpec.OmitServiceFrontend = false
	for _, c := range p.Containers {
		pSpec.Containers = append(pSpec.Containers, c.ToContainerSpec())
	}
	pSpec.Service = p.caaSSpecV2.Service
	pSpec.ConfigMaps = p.caaSSpecV2.ConfigMaps

	if p.caaSSpecV2.ServiceAccount != nil {
		pSpec.ServiceAccount = p.caaSSpecV2.ServiceAccount.ToLatest()
	}
	if p.K8sPodSpecV2.KubernetesResources != nil {
		pSpec.ProviderPod = &K8sPodSpec{
			KubernetesResources: p.K8sPodSpecV2.KubernetesResources.toLatest(),
		}
	}
	return pSpec
}

// K8sPodSpecV2 is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type K8sPodSpecV2 struct {
	// k8s resources.
	KubernetesResources *KubernetesResourcesV2 `json:"kubernetesResources,omitempty" yaml:"kubernetesResources,omitempty"`
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

// K8sServiceAccountSpecV2 defines spec for referencing or creating a service account for version 2.
type K8sServiceAccountSpecV2 struct {
	Name                       string `json:"name" yaml:"name"`
	specs.ServiceAccountSpecV2 `json:",inline" yaml:",inline"`
}

func (ksa K8sServiceAccountSpecV2) toLatest() K8sRBACResources {
	o := ksa.ServiceAccountSpecV2.ToLatest()
	o.SetName(ksa.Name)
	return K8sRBACResources{
		ServiceAccounts: []K8sServiceAccountSpec{
			{
				Name: o.GetName(),
				ServiceAccountSpecV3: specs.ServiceAccountSpecV3{
					AutomountServiceAccountToken: o.AutomountServiceAccountToken,
					Roles:                        o.Roles,
				},
			},
		},
	}
}

// Validate returns an error if the spec is not valid.
func (ksa K8sServiceAccountSpecV2) Validate() error {
	if ksa.Name == "" {
		return errors.New("service account name is missing")
	}
	return errors.Trace(ksa.ServiceAccountSpecV2.Validate())
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
	if err := validateLabels(ing.Labels); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// KubernetesResourcesV2 is the k8s related resources for version 2.
type KubernetesResourcesV2 struct {
	Pod *PodSpec `json:"pod,omitempty" yaml:"pod,omitempty"`

	Secrets                   []Secret                                                     `json:"secrets" yaml:"secrets"`
	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `json:"customResourceDefinitions,omitempty" yaml:"customResourceDefinitions,omitempty"`
	CustomResources           map[string][]unstructured.Unstructured                       `json:"customResources,omitempty" yaml:"customResources,omitempty"`

	MutatingWebhookConfigurations   map[string][]admissionregistration.MutatingWebhook   `json:"mutatingWebhookConfigurations,omitempty" yaml:"mutatingWebhookConfigurations,omitempty"`
	ValidatingWebhookConfigurations map[string][]admissionregistration.ValidatingWebhook `json:"validatingWebhookConfigurations,omitempty" yaml:"validatingWebhookConfigurations,omitempty"`

	ServiceAccounts  []K8sServiceAccountSpecV2 `json:"serviceAccounts,omitempty" yaml:"serviceAccounts,omitempty"`
	IngressResources []K8sIngressSpec          `json:"ingressResources,omitempty" yaml:"ingressResources,omitempty"`
}

func validateCustomResourceDefinitionV2(name string, crd apiextensionsv1beta1.CustomResourceDefinitionSpec) error {
	if crd.Scope != apiextensionsv1beta1.NamespaceScoped && crd.Scope != apiextensionsv1beta1.ClusterScoped {
		return errors.NewNotSupported(nil,
			fmt.Sprintf("custom resource definition %q scope %q is not supported, please use %q or %q scope",
				name, crd.Scope, apiextensionsv1beta1.NamespaceScoped, apiextensionsv1beta1.ClusterScoped),
		)
	}
	return nil
}

func (krs *KubernetesResourcesV2) toLatest() *KubernetesResources {
	out := &KubernetesResources{
		Pod:                             krs.Pod,
		Secrets:                         krs.Secrets,
		CustomResourceDefinitions:       customResourceDefinitionsToLatest(krs.CustomResourceDefinitions),
		CustomResources:                 krs.CustomResources,
		MutatingWebhookConfigurations:   krs.MutatingWebhookConfigurations,
		ValidatingWebhookConfigurations: krs.ValidatingWebhookConfigurations,
		IngressResources:                krs.IngressResources,
	}
	for _, sa := range krs.ServiceAccounts {
		rbacSources := sa.toLatest()
		out.ServiceAccounts = append(out.ServiceAccounts, rbacSources.ServiceAccounts...)
	}
	return out
}

func customResourceDefinitionsToLatest(crds map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec) (out []K8sCustomResourceDefinitionSpec) {
	for name, crd := range crds {
		out = append(out, K8sCustomResourceDefinitionSpec{
			Name: name,
			Spec: crd,
		})
	}
	return out
}

// Validate is defined on ProviderPod.
func (krs *KubernetesResourcesV2) Validate() error {
	for k, crd := range krs.CustomResourceDefinitions {
		if err := validateCustomResourceDefinitionV2(k, crd); err != nil {
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
