// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
	core "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/juju/juju/caas"
)

type caasContainerSpec caas.ContainerSpec

type k8sContainer struct {
	caasContainerSpec `json:",inline"`
	*K8sContainerSpec `json:",inline"`
}

type k8sContainers struct {
	Containers                []k8sContainer                                               `json:"containers"`
	InitContainers            []k8sContainer                                               `json:"initContainers"`
	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `yaml:"customResourceDefinitions,omitempty"`
}

// K8sContainerSpec is a subset of v1.Container which defines
// attributes we expose for charms to set.
type K8sContainerSpec struct {
	LivenessProbe   *core.Probe           `json:"livenessProbe,omitempty"`
	ReadinessProbe  *core.Probe           `json:"readinessProbe,omitempty"`
	SecurityContext *core.SecurityContext `json:"securityContext,omitempty"`
	ImagePullPolicy core.PullPolicy       `json:"imagePullPolicy,omitempty"`
}

// Validate is defined on ProviderContainer.
func (*K8sContainerSpec) Validate() error {
	return nil
}

type caasPodSpec caas.PodSpec

type k8sPod struct {
	caasPodSpec `json:",inline"`
	*K8sPodSpec `json:",inline"`
}

// K8sServiceSpec contains attributes to be set on v1.Service when
// the application is deployed.
type K8sServiceSpec struct {
	Annotations map[string]string `json:"annotations,omitempty"`
}

// K8sPodSpec is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type K8sPodSpec struct {
	ServiceAccountName            string                   `json:"serviceAccountName,omitempty"`
	RestartPolicy                 core.RestartPolicy       `json:"restartPolicy,omitempty"`
	TerminationGracePeriodSeconds *int64                   `json:"terminationGracePeriodSeconds,omitempty"`
	ActiveDeadlineSeconds         *int64                   `json:"activeDeadlineSeconds,omitempty"`
	DNSPolicy                     core.DNSPolicy           `json:"dnsPolicy,omitempty"`
	AutomountServiceAccountToken  *bool                    `json:"automountServiceAccountToken,omitempty"`
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
func (*K8sPodSpec) Validate() error {
	return nil
}

var boolValues = set.NewStrings(
	strings.Split("y|Y|yes|Yes|YES|n|N|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF", "|")...)

// ParsePodSpec parses a YAML file which defines how to
// configure a CAAS pod. We allow for generic container
// set up plus k8s select specific features.
func ParsePodSpec(in string) (*caas.PodSpec, error) {
	// Do the common fields.
	var spec caas.PodSpec
	if err := yaml.Unmarshal([]byte(in), &spec); err != nil {
		return nil, errors.Trace(err)
	}

	// Do the k8s pod attributes.
	var pod k8sPod
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err := decoder.Decode(&pod); err != nil {
		return nil, errors.Trace(err)
	}
	if pod.K8sPodSpec != nil {
		spec.ProviderPod = pod.K8sPodSpec
	}

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
	spec.Containers = make([]caas.ContainerSpec, len(containers.Containers))
	for i, c := range containers.Containers {
		if err := c.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
		spec.Containers[i] = containerFromK8sSpec(c)
	}
	spec.InitContainers = make([]caas.ContainerSpec, len(containers.InitContainers))
	for i, c := range containers.InitContainers {
		if err := c.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
		spec.InitContainers[i] = containerFromK8sSpec(c)
	}
	spec.CustomResourceDefinitions = containers.CustomResourceDefinitions

	return &spec, spec.Validate()
}

func quoteBoolStrings(containers []k8sContainer) {
	// Any string config values that could be interpreted as bools need to be quoted.
	for _, container := range containers {
		for k, v := range container.Config {
			strValue, ok := v.(string)
			if !ok {
				continue
			}
			if boolValues.Contains(strValue) {
				container.Config[k] = fmt.Sprintf("'%s'", strValue)
			}
		}
	}
}

func containerFromK8sSpec(c k8sContainer) caas.ContainerSpec {
	result := caas.ContainerSpec{
		ImageDetails: c.ImageDetails,
		Name:         c.Name,
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
