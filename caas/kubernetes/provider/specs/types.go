// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	core "k8s.io/api/core/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/juju/juju/caas/specs"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.specs")

type (
	// K8sPodSpec is the current k8s pod spec.
	K8sPodSpec = K8sPodSpecV2
)

type k8sContainer struct {
	specs.ContainerSpec `json:",inline"`
	*K8sContainerSpec   `json:",inline"`
}

// Validate validates k8sContainer.
func (c *k8sContainer) Validate() error {
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

func (c *k8sContainer) ToContainerSpec() specs.ContainerSpec {
	quoteBoolStrings(c)
	result := specs.ContainerSpec{
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

// K8sContainerSpec is a subset of v1.Container which defines
// attributes we expose for charms to set.
type K8sContainerSpec struct {
	LivenessProbe   *core.Probe           `json:"livenessProbe,omitempty"`
	ReadinessProbe  *core.Probe           `json:"readinessProbe,omitempty"`
	SecurityContext *core.SecurityContext `json:"securityContext,omitempty"`
	ImagePullPolicy core.PullPolicy       `json:"imagePullPolicy,omitempty"`
}

// Validate validates K8sContainerSpec.
func (*K8sContainerSpec) Validate() error {
	return nil
}

var boolValues = set.NewStrings(
	strings.Split("y|Y|yes|Yes|YES|n|N|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF", "|")...,
)

func quoteBoolStrings(container *k8sContainer) {
	// Any string config values that could be interpreted as bools need to be quoted.
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

type k8sContainers struct {
	Containers     []k8sContainer `json:"containers"`
	InitContainers []k8sContainer `json:"initContainers"`
}

// Validate is defined on ProviderContainer.
func (cs *k8sContainers) Validate() error {
	if len(cs.Containers) == 0 {
		return errors.New("require at least one container spec")
	}
	return nil
}

func parseContainers(in string) (containers k8sContainers, err error) {
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err = decoder.Decode(&containers); err != nil {
		return containers, errors.Trace(err)
	}
	return containers, nil
}

// K8sServiceSpec contains attributes to be set on v1.Service when
// the application is deployed.
type K8sServiceSpec struct {
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ParsePodSpec parses a YAML file which defines how to
// configure a CAAS pod. We allow for generic container
// set up plus k8s select specific features.
func ParsePodSpec(in string) (*specs.PodSpec, error) {
	version, err := specs.GetVersion(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	parser, err := getParser(version)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return parser(in)
}

type parserType func(string) (*specs.PodSpec, error)

func getParser(specVersion specs.Version) (p parserType, _ error) {
	logger.Criticalf("getParser ---> %v", specVersion)
	switch specVersion {
	case specs.Version2:
		return parsePodSpecV2, nil
	case specs.VersionLegacy:
		return parsePodSpecLegacy, nil
	default:
		return nil, errors.NotSupportedf("podspec version %q", specVersion)
	}
}
