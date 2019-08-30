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
	Kubernetes          *K8sContainerSpec `json:"kubernetes,omitempty"`
}

// Validate validates k8sContainer.
func (c *k8sContainer) Validate() error {
	if err := c.ContainerSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if c.Kubernetes != nil {
		if err := c.Kubernetes.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

type k8sContainerInterface interface {
	Validate() error
	ToContainerSpec() specs.ContainerSpec
}

func (c *k8sContainer) ToContainerSpec() specs.ContainerSpec {
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
	if c.Kubernetes != nil {
		result.ProviderContainer = c.Kubernetes
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

// PodSpec is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type PodSpec struct {
	RestartPolicy                 core.RestartPolicy       `json:"restartPolicy,omitempty"`
	ActiveDeadlineSeconds         *int64                   `json:"activeDeadlineSeconds,omitempty"`
	TerminationGracePeriodSeconds *int64                   `json:"terminationGracePeriodSeconds,omitempty"`
	SecurityContext               *core.PodSecurityContext `json:"securityContext,omitempty"`
	Priority                      *int32                   `json:"priority,omitempty"`
	ReadinessGates                []core.PodReadinessGate  `json:"readinessGates,omitempty"`
	DNSPolicy                     core.DNSPolicy           `json:"dnsPolicy,omitempty"`
}

var boolValues = set.NewStrings(
	strings.Split("y|Y|yes|Yes|YES|n|N|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF", "|")...,
)

func quoteBoolStrings(config map[string]interface{}) {
	// Any string config values that could be interpreted as bools need to be quoted.
	for k, v := range config {
		strValue, ok := v.(string)
		if !ok {
			continue
		}
		if boolValues.Contains(strValue) {
			config[k] = fmt.Sprintf("'%s'", strValue)
		}
	}
}

type k8sContainers struct {
	Containers []k8sContainer `json:"containers"`
}

// Validate is defined on ProviderContainer.
func (cs *k8sContainers) Validate() error {
	if len(cs.Containers) == 0 {
		return errors.New("require at least one container spec")
	}
	return nil
}

type k8sContainersInterface interface {
	Validate() error
}

func parseContainers(in string, containerSpec k8sContainersInterface) error {
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in))
	if err := decoder.Decode(containerSpec); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(containerSpec.Validate())
}

// ParsePodSpec parses a YAML file which defines how to
// configure a CAAS pod. We allow for generic container
// set up plus k8s select specific features.
func ParsePodSpec(in string) (*specs.PodSpec, error) {
	return parsePodSpec(in, getParser)
}

//go:generate mockgen -package mocks -destination ./mocks/parsers_mock.go github.com/juju/juju/caas/kubernetes/provider/specs PodSpecConverter
func parsePodSpec(
	in string,
	getParser func(specVersion specs.Version) (parserType, error),
) (*specs.PodSpec, error) {
	version, err := specs.GetVersion(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	parser, err := getParser(version)
	if err != nil {
		return nil, errors.Trace(err)
	}
	spec, err := parser(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err = spec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return spec.ToLatest(), nil
}

type parserType func(string) (PodSpecConverter, error)

// PodSpecConverter defines methods to validate and convert a specific version of podspec to latest version.
type PodSpecConverter interface {
	Validate() error
	ToLatest() *specs.PodSpec
}

func getParser(specVersion specs.Version) (parserType, error) {
	switch specVersion {
	case specs.Version2:
		return parsePodSpecV2, nil
	case specs.VersionLegacy:
		return parsePodSpecLegacy, nil
	default:
		return nil, errors.NewNotSupported(nil, fmt.Sprintf("latest supported version %d, but got podspec version %d", specs.CurrentVersion, specVersion))
	}
}
