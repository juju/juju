// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"strings"

	"github.com/juju/errors"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	"github.com/juju/juju/caas/specs"
)

type k8sContainerLegacy struct {
	specs.ContainerSpecV2 `json:",inline" yaml:",inline"`
	*K8sContainerSpec     `json:",inline" yaml:",inline"`
}

// Validate validates k8sContainerLegacy.
func (c *k8sContainerLegacy) Validate() error {
	if err := c.ContainerSpecV2.Validate(); err != nil {
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
	if c.K8sContainerSpec != nil {
		result.ProviderContainer = c.K8sContainerSpec
	}
	return result
}

type k8sContainersLegacy struct {
	Containers     []k8sContainerLegacy `json:"containers" yaml:"containers"`
	InitContainers []k8sContainerLegacy `json:"initContainers" yaml:"initContainers"`
}

// Validate is defined on ProviderContainer.
func (cs *k8sContainersLegacy) Validate() error {
	if len(cs.Containers) == 0 {
		return errors.New("require at least one container spec")
	}
	for _, c := range cs.Containers {
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	for _, c := range cs.InitContainers {
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// k8sPodSpecLegacy is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type k8sPodSpecLegacy struct {
	PodSpec                      `json:",inline" yaml:",inline"`
	ServiceAccountName           string `json:"serviceAccountName,omitempty" yaml:"serviceAccountName,omitempty"`
	AutomountServiceAccountToken *bool  `json:"automountServiceAccountToken,omitempty" yaml:"automountServiceAccountToken,omitempty"`

	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `yaml:"customResourceDefinitions,omitempty"`
}

// Validate is defined on ProviderPod.
func (ksl *k8sPodSpecLegacy) Validate() error {
	for k, crd := range ksl.CustomResourceDefinitions {
		if err := validateCustomResourceDefinitionV2(k, crd); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

type caaSSpecLegacy = specs.PodSpecLegacy

type podSpecLegacy struct {
	caaSSpecLegacy      `json:",inline" yaml:",inline"`
	k8sPodSpecLegacy    `json:",inline" yaml:",inline"`
	k8sContainersLegacy `json:",inline" yaml:",inline"`
}

// Validate is defined on ProviderPod.
func (p podSpecLegacy) Validate() error {
	if err := p.k8sPodSpecLegacy.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.k8sContainersLegacy.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p podSpecLegacy) ToLatest() *specs.PodSpec {
	pSpec := &specs.PodSpec{}
	pSpec.Version = specs.CurrentVersion
	pSpec.OmitServiceFrontend = p.caaSSpecLegacy.OmitServiceFrontend
	if pSpec.OmitServiceFrontend {
		logger.Warningf("OmitServiceFrontend will be deprecated in v2.")
	}
	pSpec.Service = p.caaSSpecLegacy.Service
	pSpec.ConfigMaps = p.caaSSpecLegacy.ConfigMaps

	for _, c := range p.k8sContainersLegacy.Containers {
		c.Init = false
		pSpec.Containers = append(pSpec.Containers, c.ToContainerSpec())
	}
	for _, c := range p.k8sContainersLegacy.InitContainers {
		c.Init = true
		pSpec.Containers = append(pSpec.Containers, c.ToContainerSpec())
	}

	if p.k8sPodSpecLegacy.ServiceAccountName != "" {
		// we ignore service account stuff in legacy version.
		logger.Warningf("service account is not supported in legacy version, please use v2.")
	}

	iPodSpec := &PodSpec{
		RestartPolicy:                 p.k8sPodSpecLegacy.RestartPolicy,
		ActiveDeadlineSeconds:         p.k8sPodSpecLegacy.ActiveDeadlineSeconds,
		TerminationGracePeriodSeconds: p.k8sPodSpecLegacy.TerminationGracePeriodSeconds,
		SecurityContext:               p.k8sPodSpecLegacy.SecurityContext,
		ReadinessGates:                p.k8sPodSpecLegacy.ReadinessGates,
		DNSPolicy:                     p.k8sPodSpecLegacy.DNSPolicy,
	}
	if !iPodSpec.IsEmpty() || p.k8sPodSpecLegacy.CustomResourceDefinitions != nil {
		pSpec.ProviderPod = &K8sPodSpec{
			KubernetesResources: &KubernetesResources{
				CustomResourceDefinitions: customResourceDefinitionsToLatest(p.k8sPodSpecLegacy.CustomResourceDefinitions),
				Pod:                       iPodSpec,
			},
		}
	}
	return pSpec
}

func parsePodSpecLegacy(in string) (_ PodSpecConverter, err error) {
	// Do the common fields.
	var spec podSpecLegacy
	decoder := newStrictYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err = decoder.Decode(&spec); err != nil {
		return nil, errors.Trace(err)
	}
	return &spec, nil
}
