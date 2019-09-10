// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

// CurrentVersion is the latest version of pod spec.
const CurrentVersion Version = Version2

// PodSpec is the current version of pod spec.
type PodSpec = PodSpecV2

// FileSet defines a set of files to mount
// into the container.
type FileSet struct {
	Name      string            `yaml:"name" json:"name"`
	MountPath string            `yaml:"mountPath" json:"mountPath"`
	Files     map[string]string `yaml:"files" json:"files"`
}

// Validate validates FileSet.
func (fs *FileSet) Validate() error {
	if fs.Name == "" {
		return errors.New("file set name is missing")
	}
	if fs.MountPath == "" {
		return errors.Errorf("mount path is missing for file set %q", fs.Name)
	}
	return nil
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

// PullPolicy describes a policy for if/when to pull a container image.
type PullPolicy string

// ContainerSpec defines the data values used to configure
// a container on the CAAS substrate.
type ContainerSpec struct {
	Name string `yaml:"name"`
	Init bool   `yaml:"init,omitempty"`
	// Image is deprecated in preference to using ImageDetails.
	Image        string          `yaml:"image,omitempty"`
	ImageDetails ImageDetails    `yaml:"imageDetails"`
	Ports        []ContainerPort `yaml:"ports,omitempty"`

	Command    []string `yaml:"command,omitempty"`
	Args       []string `yaml:"args,omitempty"`
	WorkingDir string   `yaml:"workingDir,omitempty"`

	Config map[string]interface{} `yaml:"config,omitempty"`
	Files  []FileSet              `yaml:"files,omitempty"`

	ImagePullPolicy PullPolicy `json:"imagePullPolicy,omitempty"`

	// ProviderContainer defines config which is specific to a substrate, eg k8s
	ProviderContainer `yaml:"-"`
}

// ProviderContainer defines a provider specific container.
type ProviderContainer interface {
	Validate() error
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
		if err := fs.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	if spec.ProviderContainer != nil {
		return spec.ProviderContainer.Validate()
	}
	return nil
}

// ServiceSpec contains attributes to be set on v1.Service when
// the application is deployed.
type ServiceSpec struct {
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Version describes pod spec version type.
type Version int32

// PodSpecVersion indicates the version of the podspec.
type PodSpecVersion struct {
	Version Version `json:"version,omitempty"`
}

// ConfigMap describes the format of configmap resource.
type ConfigMap map[string]string

// podSpecBase defines the data values used to configure
// a pod on the CAAS substrate.
type podSpecBase struct {
	PodSpecVersion `yaml:",inline"`

	// TODO(caas): remove OmitServiceFrontend later once we deprecate legacy version.
	// Keep it for now because we have to combine it with the ServerType (from metadata.yaml).
	OmitServiceFrontend bool `json:"omitServiceFrontend" yaml:"omitServiceFrontend"`

	Service    *ServiceSpec         `json:"service,omitempty" yaml:"service,omitempty"`
	ConfigMaps map[string]ConfigMap `json:"configmaps,omitempty" yaml:"configmaps,omitempty"`

	Containers []ContainerSpec `json:"containers" yaml:"containers"`

	// ProviderPod defines config which is specific to a substrate, eg k8s
	ProviderPod `json:"-" yaml:"-"`
}

// ProviderPod defines a provider specific pod.
type ProviderPod interface {
	Validate() error
}

// Validate returns an error if the spec is not valid.
func (spec *podSpecBase) Validate(ver Version) error {
	if spec.Version != ver {
		return errors.NewNotValid(nil, fmt.Sprintf("expected version %d, but found %d", ver, spec.Version))
	}

	for _, c := range spec.Containers {
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}
	}

	if spec.ProviderPod != nil {
		return spec.ProviderPod.Validate()
	}
	return nil
}

// GetVersion picks the version from pod spec string.
func GetVersion(strSpec string) (Version, error) {
	var versionSpec PodSpecVersion
	if err := yaml.Unmarshal([]byte(strSpec), &versionSpec); err != nil {
		return VersionLegacy, errors.Trace(err)
	}
	return versionSpec.Version, nil
}
