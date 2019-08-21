// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	// "gopkg.in/yaml.v2"
	// core "k8s.io/api/core/v1"
	// apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	// k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	// "github.com/juju/juju/caas"
	"github.com/juju/juju/caas/specs"
)

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

func getParser(specVersion specs.Version) (parserType, error) {
	switch specVersion {
	case specs.Version2:
		return parsePodSpecV2, nil
	case specs.VersionLegacy:
		return parsePodSpecLegacy, nil
	}
	return nil, errors.NotSupportedf("podspec version %q", specVersion)
}
