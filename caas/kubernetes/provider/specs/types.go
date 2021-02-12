// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/core/annotations"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.specs")

type (
	// K8sPodSpec is the current k8s pod spec.
	K8sPodSpec = K8sPodSpecV3
)

type k8sContainer struct {
	specs.ContainerSpec `json:",inline" yaml:",inline"`
	Kubernetes          *K8sContainerSpec `json:"kubernetes,omitempty" yaml:"kubernetes,omitempty"`
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

func (c *k8sContainer) ToContainerSpec() specs.ContainerSpec {
	result := specs.ContainerSpec{
		ImageDetails:    c.ImageDetails,
		Name:            c.Name,
		Init:            c.Init,
		Image:           c.Image,
		Ports:           c.Ports,
		Command:         c.Command,
		Args:            c.Args,
		WorkingDir:      c.WorkingDir,
		EnvConfig:       c.EnvConfig,
		VolumeConfig:    c.VolumeConfig,
		ImagePullPolicy: c.ImagePullPolicy,
	}
	if c.Kubernetes != nil {
		result.ProviderContainer = c.Kubernetes
	}
	return result
}

// K8sContainerSpec is a subset of v1.Container which defines
// attributes we expose for charms to set.
type K8sContainerSpec struct {
	LivenessProbe   *core.Probe           `json:"livenessProbe,omitempty" yaml:"livenessProbe,omitempty"`
	ReadinessProbe  *core.Probe           `json:"readinessProbe,omitempty" yaml:"readinessProbe,omitempty"`
	StartupProbe    *core.Probe           `json:"startupProbe,omitempty" yaml:"startupProbe,omitempty"`
	SecurityContext *core.SecurityContext `json:"securityContext,omitempty" yaml:"securityContext,omitempty"`
}

// Validate validates K8sContainerSpec.
func (*K8sContainerSpec) Validate() error {
	return nil
}

// PodSpecWithAnnotations wraps a k8s podspec to add annotations and labels.
type PodSpecWithAnnotations struct {
	Labels      map[string]string
	Annotations annotations.Annotation
	core.PodSpec
}

// PodSpec is a subset of v1.PodSpec which defines
// attributes we expose for charms to set.
type PodSpec struct {
	Labels                        map[string]string        `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations                   annotations.Annotation   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	RestartPolicy                 core.RestartPolicy       `json:"restartPolicy,omitempty" yaml:"restartPolicy,omitempty"`
	ActiveDeadlineSeconds         *int64                   `json:"activeDeadlineSeconds,omitempty" yaml:"activeDeadlineSeconds,omitempty"`
	TerminationGracePeriodSeconds *int64                   `json:"terminationGracePeriodSeconds,omitempty" yaml:"terminationGracePeriodSeconds,omitempty"`
	SecurityContext               *core.PodSecurityContext `json:"securityContext,omitempty" yaml:"securityContext,omitempty"`
	ReadinessGates                []core.PodReadinessGate  `json:"readinessGates,omitempty" yaml:"readinessGates,omitempty"`
	DNSPolicy                     core.DNSPolicy           `json:"dnsPolicy,omitempty" yaml:"dnsPolicy,omitempty"`
	HostNetwork                   bool                     `json:"hostNetwork,omitempty" yaml:"hostNetwork,omitempty"`
	HostPID                       bool                     `json:"hostPID,omitempty" yaml:"hostPID,omitempty"`
	PriorityClassName             string                   `json:"priorityClassName,omitempty"`
	Priority                      *int32                   `json:"priority,omitempty"`
}

// IsEmpty checks if PodSpec is empty or not.
func (ps PodSpec) IsEmpty() bool {
	return ps.RestartPolicy == "" &&
		ps.ActiveDeadlineSeconds == nil &&
		ps.TerminationGracePeriodSeconds == nil &&
		ps.SecurityContext == nil &&
		len(ps.ReadinessGates) == 0 &&
		len(ps.Labels) == 0 &&
		len(ps.Annotations) == 0 &&
		ps.DNSPolicy == ""
}

type k8sContainers struct {
	Containers []k8sContainer `json:"containers" yaml:"containers"`
}

// Validate is defined on ProviderContainer.
func (cs *k8sContainers) Validate() error {
	if len(cs.Containers) == 0 {
		return errors.New("require at least one container spec")
	}
	for _, c := range cs.Containers {
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func validateLabels(labels map[string]string) error {
	for k, v := range labels {
		if errs := validation.IsQualifiedName(k); len(errs) != 0 {
			return errors.NotValidf("label key %q: %s", k, strings.Join(errs, "; "))
		}
		if errs := validation.IsValidLabelValue(v); len(errs) != 0 {
			return errors.NotValidf("label value: %q: at key: %q: %s", v, k, strings.Join(errs, "; "))
		}
	}
	return nil
}

// ParseRawK8sSpec parses a k8s format of YAML file which defines how to
// configure a CAAS pod. We allow for generic container
// set up plus k8s select specific features.
func ParseRawK8sSpec(in string) (string, error) {
	// TODO(caas): implement raw k8s spec parser.
	return in, nil
}

// ParsePodSpec parses a YAML file which defines how to
// configure a CAAS pod. We allow for generic container
// set up plus k8s select specific features.
func ParsePodSpec(in string) (*specs.PodSpec, error) {
	return parsePodSpec(in, getParser)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination ./mocks/parsers_mock.go github.com/juju/juju/caas/kubernetes/provider/specs PodSpecConverter
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
	k8sspec, err := parser(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err = k8sspec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	caaSSpec := k8sspec.ToLatest()
	if err := caaSSpec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return caaSSpec, nil
}

type parserType func(string) (PodSpecConverter, error)

// PodSpecConverter defines methods to validate and convert a specific version of podspec to latest version.
type PodSpecConverter interface {
	Validate() error
	ToLatest() *specs.PodSpec
}

func getParser(specVersion specs.Version) (parserType, error) {
	switch specVersion {
	case specs.Version3:
		return parsePodSpecV3, nil
	case specs.Version2:
		return parsePodSpecV2, nil
	case specs.VersionLegacy:
		return parsePodSpecLegacy, nil
	default:
		return nil, errors.NewNotSupported(nil, fmt.Sprintf("latest supported version %d, but got podspec version %d", specs.CurrentVersion, specVersion))
	}
}

// IntOrStringToK8s converts IntOrString to k8s version.
func IntOrStringToK8s(in specs.IntOrString) *intstr.IntOrString {
	o := intstr.Parse(in.String())
	return &o
}
