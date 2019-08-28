// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"strings"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/juju/juju/caas/specs"
)

type podSpecV2 struct {
	caaSSpec specs.PodSpecV2
	k8sSpec  K8sPodSpecV2
}

// Validate is defined on ProviderPod.
func (p podSpecV2) Validate() error {
	if err := p.caaSSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.k8sSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p podSpecV2) ToLatest() *specs.PodSpec {
	pSpec := &specs.PodSpec{}
	pSpec.Version = specs.CurrentVersion
	pSpec.OmitServiceFrontend = p.caaSSpec.OmitServiceFrontend
	pSpec.Containers = p.caaSSpec.Containers
	pSpec.Service = p.caaSSpec.Service
	pSpec.ConfigMaps = p.caaSSpec.ConfigMaps
	pSpec.ServiceAccount = p.caaSSpec.ServiceAccount
	pSpec.ProviderPod = &p.k8sSpec
	return pSpec
}

// K8sPodSpecV2 is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type K8sPodSpecV2 struct {
	// core pod spec.
	Pod *podSpec `json:"pod,omitempty"`

	// k8s resources.
	KubernetesResources *KubernetesResources `json:"kubernetesResources,omitempty"`
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

// KubernetesResources is the k8s related resources.
type KubernetesResources struct {
	// TODO: move secret, cm etc here for V2 ??????????
	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `json:"customResourceDefinitions,omitempty" yaml:"customResourceDefinitions,omitempty"`
}

// Validate is defined on ProviderPod.
func (krs *KubernetesResources) Validate() error {
	return nil
}

func parsePodSpecV2(in string) (_ *specs.PodSpec, err error) {
	// Do the common fields.
	var spec podSpecV2
	if err = yaml.Unmarshal([]byte(in), &spec.caaSSpec); err != nil {
		return nil, errors.Trace(err)
	}
	logger.Criticalf(
		"spec.caaSSpec.ServiceAccount -----> %#v",
		spec.caaSSpec.ServiceAccount,
	)

	// Do the k8s pod attributes.
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err = decoder.Decode(&spec.k8sSpec); err != nil {
		return nil, errors.Trace(err)
	}
	if spec.k8sSpec.KubernetesResources != nil {
		logger.Criticalf(
			"spec.k8sSpec.KubernetesResources.CustomResourceDefinitions -----> %#v",
			spec.k8sSpec.KubernetesResources.CustomResourceDefinitions["tfjobs.kubeflow.org"].Validation,
		)
	}

	// Do the k8s containers.
	var containers k8sContainers
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

	if err = spec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return spec.ToLatest(), nil
}
