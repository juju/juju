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

// FileSet defines a set of files to mount
// into the container.
type FileSet struct {
	Name      string            `yaml:"name"`
	MountPath string            `yaml:"mount-path"`
	Files     map[string]string `yaml:"files"`
}

// ContainerSpec defines the data values used to configure
// a container on the CAAS substrate.
type ContainerSpec struct {
	Name      string            `yaml:"name"`
	ImageName string            `yaml:"image-name"`
	Ports     []ContainerPort   `yaml:"ports,omitempty"`
	Config    map[string]string `yaml:"config,omitempty"`
	Files     []FileSet         `yaml:"files,omitempty"`
}

// PodSpec defines the data values used to configure
// a pod on the CAAS substrate.
type PodSpec struct {
	Containers          []ContainerSpec `yaml:"containers"`
	OmitServiceFrontend bool            `yaml:"omit-service-frontend"`
}

// ParsePodSpec parses a YAML string into a PodSpec struct.
func ParsePodSpec(in string) (*PodSpec, error) {
	var spec PodSpec
	if err := yaml.Unmarshal([]byte(in), &spec); err != nil {
		return nil, errors.Trace(err)
	}
	if len(spec.Containers) == 0 {
		return nil, errors.New("require at least one pod spec")
	}
	for _, container := range spec.Containers {
		if container.Name == "" {
			return nil, errors.New("spec name is missing")
		}
		if container.ImageName == "" {
			return nil, errors.New("spec image name is missing")
		}
		for _, fs := range container.Files {
			if fs.Name == "" {
				return nil, errors.New("file set name is missing")
			}
			if fs.MountPath == "" {
				return nil, errors.Errorf("mount path is missing for file set %q", fs.Name)
			}
		}
	}
	return &spec, nil
}
