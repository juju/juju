// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import "github.com/juju/errors"

// FileSet defines a set of files to mount
// into the container.
type FileSet struct {
	Name      string            `yaml:"name" json:"name"`
	MountPath string            `yaml:"mountPath" json:"mountPath"`
	Files     map[string]string `yaml:"files" json:"files"`
}

// ContainerPort defines a port on a container.
type ContainerPort struct {
	Name          string `yaml:"name,omitempty" json:"name,omitempty"`
	ContainerPort int32  `yaml:"containerPort" json:"containerPort"`
	Protocol      string `yaml:"protocol" json:"protocol"`
}

// ProviderContainer defines a provider specific container.
type ProviderContainer interface {
	Validate() error
}

// ContainerSpec defines the data values used to configure
// a container on the CAAS substrate.
type ContainerSpec struct {
	Name  string          `yaml:"name"`
	Image string          `yaml:"image,omitempty"`
	Ports []ContainerPort `yaml:"ports,omitempty"`

	Config map[string]string `yaml:"config,omitempty"`
	Files  []FileSet         `yaml:"files,omitempty"`

	// ProviderContainer defines config which is specific to a substrate, eg k8s
	ProviderContainer `yaml:"-"`
}

// PodSpec defines the data values used to configure
// a pod on the CAAS substrate.
type PodSpec struct {
	Containers          []ContainerSpec `yaml:"-"`
	OmitServiceFrontend bool            `yaml:"omitServiceFrontend"`
}

// Validate returns an error if the spec is not valid.
func (spec *PodSpec) Validate() error {
	for _, c := range spec.Containers {
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Validate is defined on ProviderContainer.
func (spec *ContainerSpec) Validate() error {
	if spec.Name == "" {
		return errors.New("spec name is missing")
	}
	if spec.Image == "" {
		return errors.New("spec image is missing")
	}
	for _, fs := range spec.Files {
		if fs.Name == "" {
			return errors.New("file set name is missing")
		}
		if fs.MountPath == "" {
			return errors.Errorf("mount path is missing for file set %q", fs.Name)
		}
	}
	if spec.ProviderContainer != nil {
		return spec.ProviderContainer.Validate()
	}
	return nil
}
