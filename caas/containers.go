// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

// ContainerPort defines the attributes used to configure
// an open port for the container.
type ContainerPort struct {
	ContainerPort int    `yaml:"container-port"`
	Protocol      string `yaml:"protocol"`
}

// ContainerSpec defines the data values used to configure
// a container on the CAAS substrate.
type ContainerSpec struct {
	Name      string            `yaml:"name"`
	ImageName string            `yaml:"image-name"`
	Ports     []ContainerPort   `yaml:"ports,omitempty"`
	Config    map[string]string `yaml:"config,omitempty"`
}

// ParseContainerSpec parses a YAML string into a ContainerSpec struct.
func ParseContainerSpec(in string) (*ContainerSpec, error) {
	var spec ContainerSpec
	if err := yaml.Unmarshal([]byte(in), &spec); err != nil {
		return nil, errors.Trace(err)
	}
	if spec.Name == "" {
		return nil, errors.New("spec name is missing")
	}
	if spec.ImageName == "" {
		return nil, errors.New("spec image name is missing")
	}
	return &spec, nil
}
