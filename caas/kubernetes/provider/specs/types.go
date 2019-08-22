// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	// "gopkg.in/yaml.v2"
	core "k8s.io/api/core/v1"
	// apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	// k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	// "github.com/juju/juju/caas"
	"github.com/juju/juju/caas/specs"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.specs")

type (
	caasContainerSpec specs.ContainerSpec
	// K8sPodSpec is the current k8s pod spec.
	K8sPodSpec = K8sPodSpecV2
)

type k8sContainer struct {
	caasContainerSpec `json:",inline"`
	*K8sContainerSpec `json:",inline"`
}

type k8sContainers struct {
	Containers     []k8sContainer `json:"containers"`
	InitContainers []k8sContainer `json:"initContainers"`
	// CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `yaml:"customResourceDefinitions,omitempty"`
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

// K8sServiceSpec contains attributes to be set on v1.Service when
// the application is deployed.
type K8sServiceSpec struct {
	Annotations map[string]string `json:"annotations,omitempty"`
}

var boolValues = set.NewStrings(
	strings.Split("y|Y|yes|Yes|YES|n|N|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF", "|")...,
)

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

func containerFromK8sSpec(c k8sContainer) specs.ContainerSpec {
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
