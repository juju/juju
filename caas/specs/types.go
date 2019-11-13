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
	Name      string            `json:"name" yaml:"name"`
	MountPath string            `json:"mountPath" yaml:"mountPath"`
	Files     map[string]string `json:"files" yaml:"files"`
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
	Name          string `json:"name,omitempty" yaml:"name,omitempty"`
	ContainerPort int32  `json:"containerPort" yaml:"containerPort"`
	Protocol      string `json:"protocol" yaml:"protocol"`
}

// ImageDetails defines all details required to pull a docker image from any registry
type ImageDetails struct {
	ImagePath string `json:"imagePath" yaml:"imagePath"`
	Username  string `json:"username,omitempty" yaml:"username,omitempty"`
	Password  string `json:"password,omitempty" yaml:"password,omitempty"`
}

// PullPolicy describes a policy for if/when to pull a container image.
type PullPolicy string

// ContainerSpec defines the data values used to configure
// a container on the CAAS substrate.
type ContainerSpec struct {
	Name string `json:"name" yaml:"name"`
	Init bool   `json:"init,omitempty" yaml:"init,omitempty"`
	// Image is deprecated in preference to using ImageDetails.
	Image        string          `json:"image,omitempty" yaml:"image,omitempty"`
	ImageDetails ImageDetails    `json:"imageDetails" yaml:"imageDetails"`
	Ports        []ContainerPort `json:"ports,omitempty" yaml:"ports,omitempty"`

	Command    []string `json:"command,omitempty" yaml:"command,omitempty"`
	Args       []string `json:"args,omitempty" yaml:"args,omitempty"`
	WorkingDir string   `json:"workingDir,omitempty" yaml:"workingDir,omitempty"`

	Config map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
	Files  []FileSet              `json:"files,omitempty" yaml:"files,omitempty"`

	ImagePullPolicy PullPolicy `json:"imagePullPolicy,omitempty" yaml:"imagePullPolicy,omitempty"`

	// ProviderContainer defines config which is specific to a substrate, eg k8s
	ProviderContainer `json:"-" yaml:"-"`
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

// ScalePolicyType defines the policy for creating or terminating pods under a service.
type ScalePolicyType string

// Validate returns an error if the spec is not valid.
func (spt ScalePolicyType) Validate() error {
	if spt == "" {
		return nil
	}
	for _, v := range supportedPolicies {
		if spt == v {
			return nil
		}
	}
	return errors.NotSupportedf("%v", spt)
}

const (
	// ParallelScale will create and delete pods as soon as the
	// replica count is changed, and will not wait for pods to be ready or complete
	// termination.
	ParallelScale ScalePolicyType = "parallel"

	// SerialScale will create pods in strictly increasing order on
	// scale up and strictly decreasing order on scale down, progressing only when
	// the previous pod is ready or terminated. At most one pod will be changed
	// at any time.
	SerialScale = "serial"
)

var supportedPolicies = []ScalePolicyType{
	ParallelScale,
	SerialScale,
}

// ServiceSpec contains attributes to be set on v1.Service when
// the application is deployed.
type ServiceSpec struct {
	ScalePolicy ScalePolicyType   `json:"scalePolicy,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Validate returns an error if the spec is not valid.
func (ss ServiceSpec) Validate() error {
	return errors.Trace(ss.ScalePolicy.Validate())
}

// Version describes pod spec version type.
type Version int32

// PodSpecVersion indicates the version of the podspec.
type PodSpecVersion struct {
	Version Version `json:"version,omitempty" yaml:"version,omitempty"`
}

// ConfigMap describes the format of configmap resource.
type ConfigMap map[string]string

type caasContainers struct {
	Containers []ContainerSpec
}

// Validate is defined on ProviderContainer.
func (cs *caasContainers) Validate() error {
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

// podSpecBase defines the data values used to configure a pod on the CAAS substrate.
type podSpecBase struct {
	PodSpecVersion `json:",inline" yaml:",inline"`

	// TODO(caas): remove OmitServiceFrontend later once we deprecate legacy version.
	// Keep it for now because we have to combine it with the ServerType (from metadata.yaml).
	OmitServiceFrontend bool `json:"omitServiceFrontend" yaml:"omitServiceFrontend"`

	Service    *ServiceSpec         `json:"service,omitempty" yaml:"service,omitempty"`
	ConfigMaps map[string]ConfigMap `json:"configmaps,omitempty" yaml:"configmaps,omitempty"`

	caasContainers // containers field is decoded in provider spec level.

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

	if spec.Service != nil {
		if err := spec.Service.Validate(); err != nil {
			return errors.Trace(err)
		}
	}

	if err := spec.caasContainers.Validate(); err != nil {
		return errors.Trace(err)
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
