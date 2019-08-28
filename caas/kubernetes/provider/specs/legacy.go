// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"strings"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
	core "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/juju/juju/caas/specs"
)

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
		logger.Criticalf("podSpecLegacy init container -> %+v", c)
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
		Pod: &podSpec{
			RestartPolicy:                 p.k8sSpec.RestartPolicy,
			ActiveDeadlineSeconds:         p.k8sSpec.ActiveDeadlineSeconds,
			TerminationGracePeriodSeconds: p.k8sSpec.TerminationGracePeriodSeconds,
			SecurityContext:               p.k8sSpec.SecurityContext,
			Priority:                      p.k8sSpec.Priority,
			ReadinessGates:                p.k8sSpec.ReadinessGates,
			DNSPolicy:                     p.k8sSpec.DNSPolicy,

			// TODO: should we just ignore below deprecated pod config ?????????
			// Hostname: p.k8sSpec.Hostname,
			// Subdomain: p.k8sSpec.Subdomain,
			// PriorityClassName: p.k8sSpec.PriorityClassName,
			// DNSConfig: p.k8sSpec.DNSConfig,
		},
	}
	return pSpec
}

// K8sPodSpecLegacy is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type K8sPodSpecLegacy struct {
	// TODO(caas): remove ServiceAccountName and AutomountServiceAccountToken in the future
	// because we have service account spec in caas.PodSpec now.
	// Keep it for now because it will be a breaking change to remove it.
	ServiceAccountName           string `json:"serviceAccountName,omitempty"`
	AutomountServiceAccountToken *bool  `json:"automountServiceAccountToken,omitempty"`

	podSpec `yaml:",inline"`
	// TODO: should we just ignore below deprecated pod config ?????????
	Hostname          string             `json:"hostname,omitempty"`
	Subdomain         string             `json:"subdomain,omitempty"`
	PriorityClassName string             `json:"priorityClassName,omitempty"`
	DNSConfig         *core.PodDNSConfig `json:"dnsConfig,omitempty"`

	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `yaml:"customResourceDefinitions,omitempty"`
}

// Validate is defined on ProviderPod.
func (*K8sPodSpecLegacy) Validate() error {
	return nil
}

type k8sContainersLegacy struct {
	k8sContainers  `yaml:",inline"`
	InitContainers []k8sContainer `json:"initContainers"`
}

// Validate is defined on ProviderContainer.
func (cs *k8sContainersLegacy) Validate() error {
	return errors.Trace(cs.k8sContainers.Validate())
}

func parsePodSpecLegacy(in string) (_ *specs.PodSpec, err error) {
	// Do the common fields.
	var spec podSpecLegacy
	if err = yaml.Unmarshal([]byte(in), &spec.caaSSpec); err != nil {
		return nil, errors.Trace(err)
	}

	// Do the k8s pod attributes.
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err = decoder.Decode(&spec.k8sSpec); err != nil {
		return nil, errors.Trace(err)
	}
	if spec.k8sSpec.CustomResourceDefinitions != nil {
		logger.Criticalf(
			"spec.k8sSpec.CustomResourceDefinitions -----> %#v",
			spec.k8sSpec.CustomResourceDefinitions["tfjobs.kubeflow.org"].Validation,
		)
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
