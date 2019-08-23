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

	// "github.com/juju/juju/caas"
	"github.com/juju/juju/caas/specs"
)

type podSpecLegacy struct {
	CaaSSpec specs.PodSpecLegacy
	K8sSpec  K8sPodSpecLegacy
}

// Validate is defined on ProviderPod.
func (p podSpecLegacy) Validate() error {
	if err := p.CaaSSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := p.K8sSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p podSpecLegacy) ToLatest() *specs.PodSpec {
	pSpec := &specs.PodSpec{}
	pSpec.Version = specs.CurrentVersion
	pSpec.OmitServiceFrontend = p.CaaSSpec.OmitServiceFrontend
	pSpec.Containers = p.CaaSSpec.Containers
	pSpec.InitContainers = p.CaaSSpec.InitContainers
	pSpec.ProviderPod = &K8sPodSpec{
		KubernetesResources: &KubernetesResources{
			ServiceAccount: &ServiceAccountSpec{
				Name:                         p.K8sSpec.ServiceAccountName,
				AutomountServiceAccountToken: p.K8sSpec.AutomountServiceAccountToken,
			},
			CustomResourceDefinitions: p.K8sSpec.CustomResourceDefinitions,
		},
		RestartPolicy:                 p.K8sSpec.RestartPolicy,
		TerminationGracePeriodSeconds: p.K8sSpec.TerminationGracePeriodSeconds,
		ActiveDeadlineSeconds:         p.K8sSpec.ActiveDeadlineSeconds,
		DNSPolicy:                     p.K8sSpec.DNSPolicy,
		SecurityContext:               p.K8sSpec.SecurityContext,
		Hostname:                      p.K8sSpec.Hostname,
		Subdomain:                     p.K8sSpec.Subdomain,
		PriorityClassName:             p.K8sSpec.PriorityClassName,
		Priority:                      p.K8sSpec.Priority,
		DNSConfig:                     p.K8sSpec.DNSConfig,
		ReadinessGates:                p.K8sSpec.ReadinessGates,
		Service:                       p.K8sSpec.Service,
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

	RestartPolicy                 core.RestartPolicy       `json:"restartPolicy,omitempty"`
	TerminationGracePeriodSeconds *int64                   `json:"terminationGracePeriodSeconds,omitempty"`
	ActiveDeadlineSeconds         *int64                   `json:"activeDeadlineSeconds,omitempty"`
	DNSPolicy                     core.DNSPolicy           `json:"dnsPolicy,omitempty"`
	SecurityContext               *core.PodSecurityContext `json:"securityContext,omitempty"`
	Hostname                      string                   `json:"hostname,omitempty"`
	Subdomain                     string                   `json:"subdomain,omitempty"`
	PriorityClassName             string                   `json:"priorityClassName,omitempty"`
	Priority                      *int32                   `json:"priority,omitempty"`
	DNSConfig                     *core.PodDNSConfig       `json:"dnsConfig,omitempty"`
	ReadinessGates                []core.PodReadinessGate  `json:"readinessGates,omitempty"`
	Service                       *K8sServiceSpec          `json:"service,omitempty"`

	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `yaml:"customResourceDefinitions,omitempty"`
}

// Validate is defined on ProviderPod.
func (*K8sPodSpecLegacy) Validate() error {
	return nil
}

func parsePodSpecLegacy(in string) (_ *specs.PodSpec, err error) {
	// Do the common fields.
	var spec podSpecLegacy
	if err = yaml.Unmarshal([]byte(in), &spec.CaaSSpec); err != nil {
		return nil, errors.Trace(err)
	}

	// Do the k8s pod attributes.
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err = decoder.Decode(&spec.K8sSpec); err != nil {
		return nil, errors.Trace(err)
	}
	if spec.K8sSpec.CustomResourceDefinitions != nil {
		logger.Criticalf(
			"spec.K8sSpec.CustomResourceDefinitions -----> %#v",
			spec.K8sSpec.CustomResourceDefinitions["tfjobs.kubeflow.org"].Validation,
		)
	}

	// Do the k8s containers.
	containers, err := parseContainers(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err = containers.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Compose the result.
	for i, c := range containers.Containers {
		if err = c.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
		spec.CaaSSpec.Containers[i] = c.ToContainerSpec()
	}
	for i, c := range containers.InitContainers {
		if err = c.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
		spec.CaaSSpec.InitContainers[i] = c.ToContainerSpec()
	}
	if err = spec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return spec.ToLatest(), nil
}
