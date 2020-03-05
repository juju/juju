// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

// CurrentVersion is the latest version of pod spec.
const CurrentVersion Version = Version3

// PodSpec is the current version of pod spec.
type PodSpec = PodSpecV3

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

// ContainerConfig describes the config used for setting up a pod container's environment variables.
type ContainerConfig map[string]interface{}

// ContainerSpecV2 defines the data values used to configure
// a container on the CAAS substrate.
type ContainerSpecV2 struct {
	Name string `json:"name" yaml:"name"`
	Init bool   `json:"init,omitempty" yaml:"init,omitempty"`
	// Image is deprecated in preference to using ImageDetails.
	Image        string          `json:"image,omitempty" yaml:"image,omitempty"`
	ImageDetails ImageDetails    `json:"imageDetails" yaml:"imageDetails"`
	Ports        []ContainerPort `json:"ports,omitempty" yaml:"ports,omitempty"`

	Command    []string `json:"command,omitempty" yaml:"command,omitempty"`
	Args       []string `json:"args,omitempty" yaml:"args,omitempty"`
	WorkingDir string   `json:"workingDir,omitempty" yaml:"workingDir,omitempty"`

	Config ContainerConfig `json:"config,omitempty" yaml:"config,omitempty"`
	Files  []FileSetV2     `json:"files,omitempty" yaml:"files,omitempty"`

	ImagePullPolicy PullPolicy `json:"imagePullPolicy,omitempty" yaml:"imagePullPolicy,omitempty"`

	// ProviderContainer defines config which is specific to a substrate, eg k8s
	ProviderContainer `json:"-" yaml:"-"`
}

// Validate is defined on ProviderContainer.
func (spec *ContainerSpecV2) Validate() error {
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

	EnvConfig    ContainerConfig `json:"envConfig,omitempty" yaml:"envConfig,omitempty"`
	VolumeConfig []FileSet       `json:"volumeConfig,omitempty" yaml:"volumeConfig,omitempty"`

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
	for _, fs := range spec.VolumeConfig {
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
	return errors.NotSupportedf("scale policy %q", spt)
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
	SerialScale ScalePolicyType = "serial"
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

type caasContainersV2 struct {
	Containers []ContainerSpecV2
}

// Validate is defined on ProviderContainer.
func (cs *caasContainersV2) Validate() error {
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

type caasContainers struct {
	Containers []ContainerSpec
}

// Validate is defined on ProviderContainer.
func (cs *caasContainers) Validate() error {
	if len(cs.Containers) == 0 {
		return errors.New("require at least one container spec")
	}
	// Same FileSet can be shared across different containers in same pod.
	// But we need ensure the two FileSets are exactly the same Volume if the FileSet.Name are same.
	uniqVols := make(map[string]FileSet)

	for _, c := range cs.Containers {
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}

		var uniqFileSets []FileSet
		isDuplicateInContainer := func(f FileSet) bool {
			for _, v := range uniqFileSets {
				if f.Equal(v) {
					return true
				}
			}
			return false
		}
		mountPaths := set.NewStrings()
		for i := range c.VolumeConfig {
			f := c.VolumeConfig[i]
			if v, ok := uniqVols[f.Name]; ok {
				if !f.EqualVolume(v) {
					return errors.NotValidf("duplicated file %q with different volume spec", f.Name)
				}
			} else {
				uniqVols[f.Name] = f
			}

			// No deplicated FileSet in same container, but it's ok in different containers in same pod.
			if isDuplicateInContainer(f) {
				return errors.NotValidf("duplicated file %q in container %q", f.Name, c.Name)
			}
			uniqFileSets = append(uniqFileSets, f)

			// Same mount path can't be mounted more than once in same container.
			if mountPaths.Contains(f.MountPath) {
				return errors.NotValidf("duplicated mount path %q in container %q", f.MountPath, c.Name)
			}
			mountPaths.Add(f.MountPath)
		}
	}
	return nil
}

// podSpecBaseV2 defines the data values used to configure a pod on the CAAS substrate.
type podSpecBaseV2 struct {
	PodSpecVersion `json:",inline" yaml:",inline"`

	// TODO(caas): remove OmitServiceFrontend later once we deprecate legacy version.
	// Keep it for now because we have to combine it with the ServerType (from metadata.yaml).
	OmitServiceFrontend bool `json:"omitServiceFrontend" yaml:"omitServiceFrontend"`

	Service    *ServiceSpec         `json:"service,omitempty" yaml:"service,omitempty"`
	ConfigMaps map[string]ConfigMap `json:"configmaps,omitempty" yaml:"configmaps,omitempty"`

	caasContainersV2 // containers field is decoded in provider spec level.

	// ProviderPod defines config which is specific to a substrate, eg k8s
	ProviderPod `json:"-" yaml:"-"`
}

// Validate returns an error if the spec is not valid.
func (spec *podSpecBaseV2) Validate(ver Version) error {
	if spec.Version != ver {
		return errors.NewNotValid(nil, fmt.Sprintf("expected version %d, but found %d", ver, spec.Version))
	}

	if spec.Service != nil {
		if err := spec.Service.Validate(); err != nil {
			return errors.Trace(err)
		}
	}

	if err := spec.caasContainersV2.Validate(); err != nil {
		return errors.Trace(err)
	}

	if spec.ProviderPod != nil {
		return spec.ProviderPod.Validate()
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
