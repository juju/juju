// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"strings"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
	core "k8s.io/api/core/v1"
	// apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	// "github.com/juju/juju/caas"
	"github.com/juju/juju/caas/specs"
)

type podSpecLegacy struct {
	CaaSSpec specs.PodSpecLegacy
	K8sSpec  K8sPodSpecLegacy
}

// Validate is defined on ProviderPod.
func (p *podSpecLegacy) Validate() error {
	if err := p.CaaSSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(p.K8sSpec.Validate())
}

func (p *podSpecLegacy) ToLatest() *specs.PodSpec {
	pSpec := &specs.PodSpec{}
	pSpec.Version = specs.CurrentVersion
	pSpec.OmitServiceFrontend = p.CaaSSpec.OmitServiceFrontend
	pSpec.Containers = p.CaaSSpec.Containers
	pSpec.InitContainers = p.CaaSSpec.InitContainers
	pSpec.ProviderPod = &K8sPodSpec{
		ServiceAccount: &ServiceAccountSpec{
			Name:                         p.K8sSpec.ServiceAccountName,
			AutomountServiceAccountToken: p.K8sSpec.AutomountServiceAccountToken,
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
		CustomResourceDefinitions:     p.CaaSSpec.CustomResourceDefinitions,
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
}

// Validate is defined on ProviderPod.
func (*K8sPodSpecLegacy) Validate() error {
	return nil
}

func parsePodSpecLegacy(in string) (*specs.PodSpec, error) {
	// Do the common fields.
	var spec podSpecLegacy
	var caasPodSpecLegacy specs.PodSpecLegacy
	if err := yaml.Unmarshal([]byte(in), &caasPodSpecLegacy); err != nil {
		return nil, errors.Trace(err)
	}
	logger.Criticalf("in ---> \n%s", in)
	logger.Criticalf("caasPodSpecLegacy -----> %#v", caasPodSpecLegacy)
	logger.Criticalf("caasPodSpecLegacy -----> %#v", caasPodSpecLegacy.CustomResourceDefinitions["tfjobs.kubeflow.org"].Validation)
	spec.CaaSSpec = caasPodSpecLegacy
	// Do the k8s pod attributes.
	var pod K8sPodSpecLegacy
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err := decoder.Decode(&pod); err != nil {
		return nil, errors.Trace(err)
	}
	spec.K8sSpec = pod
	// if pod.K8sPodSpecLegacy != nil {
	// 	spec.ProviderPod = pod.K8sPodSpecLegacy
	// }
	// 	if pod.ServiceAccount != nil {
	// 		if pod.K8sPodSpec != nil && pod.ServiceAccountName != "" {
	// 			return nil, errors.New(`
	// either use ServiceAccountName to reference existing service account or define ServiceAccount spec to create a new one`[1:])
	// 		}
	// 		spec.ServiceAccount = pod.ServiceAccount
	// 	}

	// Do the k8s containers.
	var containers k8sContainers
	decoder = k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err := decoder.Decode(&containers); err != nil {
		return nil, errors.Trace(err)
	}

	if len(containers.Containers) == 0 {
		return nil, errors.New("require at least one container spec")
	}
	quoteBoolStrings(containers.Containers)
	quoteBoolStrings(containers.InitContainers)

	// Compose the result.
	spec.CaaSSpec.Containers = make([]specs.ContainerSpec, len(containers.Containers))
	for i, c := range containers.Containers {
		if err := c.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
		spec.CaaSSpec.Containers[i] = containerFromK8sSpec(c)
	}
	spec.CaaSSpec.InitContainers = make([]specs.ContainerSpec, len(containers.InitContainers))
	for i, c := range containers.InitContainers {
		if err := c.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
		spec.CaaSSpec.InitContainers[i] = containerFromK8sSpec(c)
	}
	if err := spec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return spec.ToLatest(), nil
}
