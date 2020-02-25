// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"strings"

	"github.com/juju/errors"

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
