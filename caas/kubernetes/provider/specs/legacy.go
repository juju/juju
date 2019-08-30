// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"strings"

	"github.com/juju/errors"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/juju/juju/caas/specs"
)

type k8sContainerLegacy struct {
	specs.ContainerSpec `json:",inline"`
	*K8sContainerSpec   `json:",inline"`
}

// Validate validates k8sContainerLegacy.
func (c *k8sContainerLegacy) Validate() error {
	if err := c.ContainerSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if c.K8sContainerSpec != nil {
		if err := c.K8sContainerSpec.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *k8sContainerLegacy) ToContainerSpec() specs.ContainerSpec {
	quoteBoolStrings(c.Config)
	result := specs.ContainerSpec{
		ImageDetails: c.ImageDetails,
		Name:         c.Name,
		Init:         c.Init,
		Image:        c.Image,
		Ports:        c.Ports,
		Command:      c.Command,
		Args:         c.Args,
		WorkingDir:   c.WorkingDir,
		Config:       c.Config,
		Files:        c.Files,
	}
	if c.K8sContainerSpec != nil {
		result.ProviderContainer = c.K8sContainerSpec
	}
	return result
}

type podSpecLegacy struct {
	caaSSpec specs.PodSpecLegacy
	k8sSpec  K8sPodSpecLegacy
}

// Validate is defined on ProviderPod.
func (p podSpecLegacy) Validate() error {
	if err := p.caaSSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.k8sSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p podSpecLegacy) ToLatest() *specs.PodSpec {
	pSpec := &specs.PodSpec{}
	pSpec.Version = specs.CurrentVersion
	pSpec.OmitServiceFrontend = p.caaSSpec.OmitServiceFrontend
	pSpec.Service = p.caaSSpec.Service
	pSpec.ConfigMaps = p.caaSSpec.ConfigMaps
	pSpec.Containers = p.caaSSpec.Containers
	for _, c := range p.caaSSpec.InitContainers {
		pSpec.Containers = append(pSpec.Containers, c)
	}

	pSpec.ServiceAccount = &specs.ServiceAccountSpec{
		Name:                         p.k8sSpec.ServiceAccountName,
		AutomountServiceAccountToken: p.k8sSpec.AutomountServiceAccountToken,
	}

	pSpec.ProviderPod = &K8sPodSpec{
		KubernetesResources: &KubernetesResources{
			CustomResourceDefinitions: p.k8sSpec.CustomResourceDefinitions,
		},
		Pod: &PodSpec{
			RestartPolicy:                 p.k8sSpec.RestartPolicy,
			ActiveDeadlineSeconds:         p.k8sSpec.ActiveDeadlineSeconds,
			TerminationGracePeriodSeconds: p.k8sSpec.TerminationGracePeriodSeconds,
			SecurityContext:               p.k8sSpec.SecurityContext,
			Priority:                      p.k8sSpec.Priority,
			ReadinessGates:                p.k8sSpec.ReadinessGates,
			DNSPolicy:                     p.k8sSpec.DNSPolicy,
		},
	}
	return pSpec
}

// K8sPodSpecLegacy is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type K8sPodSpecLegacy struct {
	PodSpec                      `yaml:",inline"`
	ServiceAccountName           string `json:"serviceAccountName,omitempty"`
	AutomountServiceAccountToken *bool  `json:"automountServiceAccountToken,omitempty"`

	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `yaml:"customResourceDefinitions,omitempty"`
}

// Validate is defined on ProviderPod.
func (*K8sPodSpecLegacy) Validate() error {
	return nil
}

type k8sContainersLegacy struct {
	Containers     []k8sContainerLegacy `json:"containers"`
	InitContainers []k8sContainerLegacy `json:"initContainers"`
}

// Validate is defined on ProviderContainer.
func (cs *k8sContainersLegacy) Validate() error {
	if len(cs.Containers) == 0 {
		return errors.New("require at least one container spec")
	}
	return nil
}

func parsePodSpecLegacy(in string) (_ *specs.PodSpec, err error) {
	// Do the common fields.
	var spec podSpecLegacy

	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err = decoder.Decode(&spec.caaSSpec); err != nil {
		return nil, errors.Trace(err)
	}

	// Do the k8s pod attributes.
	decoder = k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err = decoder.Decode(&spec.k8sSpec); err != nil {
		return nil, errors.Trace(err)
	}

	// Do the k8s containers.
	var containers k8sContainersLegacy
	if err := parseContainers(in, &containers); err != nil {
		return nil, errors.Trace(err)
	}

	// Compose the result.
	for i, c := range containers.Containers {
		if err = c.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
		spec.caaSSpec.Containers[i] = c.ToContainerSpec()
	}
	for i, c := range containers.InitContainers {
		if err = c.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
		spec.caaSSpec.InitContainers[i] = c.ToContainerSpec()
	}
	if err = spec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return spec.ToLatest(), nil
}
