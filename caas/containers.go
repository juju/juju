// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/errors"
	// apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

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

// ImageDetails defines all details required to pull a docker image from any registry
type ImageDetails struct {
	ImagePath string `yaml:"imagePath" json:"imagePath"`
	Username  string `yaml:"username,omitempty" json:"username,omitempty"`
	Password  string `yaml:"password,omitempty" json:"password,omitempty"`
}

// ProviderContainer defines a provider specific container.
type ProviderContainer interface {
	Validate() error
}

// ContainerSpec defines the data values used to configure
// a container on the CAAS substrate.
type ContainerSpec struct {
	Name string `yaml:"name"`
	// Image is deprecated in preference to using ImageDetails.
	Image        string          `yaml:"image,omitempty"`
	ImageDetails ImageDetails    `yaml:"imageDetails"`
	Ports        []ContainerPort `yaml:"ports,omitempty"`

	Command    []string `yaml:"command,omitempty"`
	Args       []string `yaml:"args,omitempty"`
	WorkingDir string   `yaml:"workingDir,omitempty"`

	Config map[string]string `yaml:"config,omitempty"`
	Files  []FileSet         `yaml:"files,omitempty"`

	// ProviderContainer defines config which is specific to a substrate, eg k8s
	ProviderContainer `yaml:"-"`
}

// PodSpec defines the data values used to configure
// a pod on the CAAS substrate.
type PodSpec struct {
	Containers               []ContainerSpec          `yaml:"-"`
	OmitServiceFrontend      bool                     `yaml:"omitServiceFrontend"`
	CustomResourceDefinition CustomResourceDefinition `yaml:"crd,omitempty"`
}

// CrdObject defines the custom resource definition new resource.
type CrdObject struct {
	Name string                 `yaml:"name"`
	Spec map[string]interface{} `yaml:"spec"`
}

// CrdTemplateValidation defines the custom resource definition validation schema.
type CrdTemplateValidation struct {
	OpenAPIV3Schema map[string]interface{} `yaml:"openAPIV3Schema,omitempty"`
}

// CrdTemplate defines the custom resource definition type.
type CrdTemplate struct {
	Name       string                `yaml:"name"`
	Group      string                `yaml:"group"`
	Version    string                `yaml:"version"`
	Scope      string                `yaml:"scope"`
	Validation CrdTemplateValidation `yaml:"validation,omitempty"`
}

// CustomResourceDefinition defines the custom resource definition template and content format in podspec.
type CustomResourceDefinition struct {
	Template CrdTemplate `yaml:"template"`
	Content  CrdObject   `yaml:"content"`
}

// Validate returns an error if the crd is not valid.
func (crd *CustomResourceDefinition) Validate() error {
	// do validation.
	return nil
}

// IsPresent checks if CustomResourceDefinition is empty.
func (crd *CustomResourceDefinition) IsPresent() bool {
	return crd.Template.Name != "" && crd.Content.Name != ""
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
	if spec.Image == "" && spec.ImageDetails.ImagePath == "" {
		return errors.New("spec image details is missing")
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
