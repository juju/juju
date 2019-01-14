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
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/juju/juju/caas"
)

type caasContainerSpec caas.ContainerSpec

type k8sContainer struct {
	caasContainerSpec `json:",inline"`
	*K8sContainerSpec `json:",inline"`
}

type k8sContainers struct {
	Containers []k8sContainer `json:"containers"`
}

// K8sContainerSpec is a subset of v1.Container which defines
// attributes we expose for charms to set.
type K8sContainerSpec struct {
	LivenessProbe   *core.Probe     `json:"livenessProbe,omitempty"`
	ReadinessProbe  *core.Probe     `json:"readinessProbe,omitempty"`
	ImagePullPolicy core.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// Validate is defined on ProviderContainer.
func (*K8sContainerSpec) Validate() error {
	return nil
}

var boolValues = set.NewStrings(
	strings.Split("y|Y|yes|Yes|YES|n|N|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF", "|")...)

// parseK8sPodSpec parses a YAML file which defines how to
// configure a CAAS pod. We allow for generic container
// set up plus k8s select specific features.
func parseK8sPodSpec(in string) (*caas.PodSpec, error) {
	// Do the common fields.
	var spec caas.PodSpec
	if err := yaml.Unmarshal([]byte(in), &spec); err != nil {
		return nil, errors.Trace(err)
	}

	// Do the k8s containers.
	var containers k8sContainers
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err := decoder.Decode(&containers); err != nil {
		return nil, errors.Trace(err)
	}

	if len(containers.Containers) == 0 {
		return nil, errors.New("require at least one container spec")
	}

	// Any string config values that could be interpreted as bools need to be quoted.
	for _, container := range containers.Containers {
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

	// Compose the result.
	spec.Containers = make([]caas.ContainerSpec, len(containers.Containers))
	for i, c := range containers.Containers {
		if err := c.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
		spec.Containers[i] = caas.ContainerSpec{
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
			spec.Containers[i].ProviderContainer = c.K8sContainerSpec
		}
	}
	return &spec, nil
}
