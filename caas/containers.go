// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/errors"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
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

	Config map[string]interface{} `yaml:"config,omitempty"`
	Files  []FileSet              `yaml:"files,omitempty"`

	// ProviderContainer defines config which is specific to a substrate, eg k8s
	ProviderContainer `yaml:"-"`
}

// RBACType describes Role/Binding type.
type RBACType string

const (
	// Role is the single namespace based.
	Role RBACType = "Role"
	// ClusterRole is the all namespaces based.
	ClusterRole RBACType = "ClusterRole"

	// RoleBinding is the single namespace based.
	RoleBinding RBACType = "RoleBinding"
	// ClusterRoleBinding is the all namespaces based.
	ClusterRoleBinding RBACType = "ClusterRoleBinding"
)

var (
	// RoleTypes defines supported role types.
	RoleTypes = []RBACType{
		Role, ClusterRole,
	}
	// RoleBindingTypes defines supported role binding types.
	RoleBindingTypes = []RBACType{
		RoleBinding, ClusterRoleBinding,
	}
)

// RoleSpec defines config for referencing to or creating a Role or Cluster resource.
type RoleSpec struct {
	Name string   `yaml:"name"`
	Type RBACType `yaml:"type"`
	// We assume this is an existing Role/ClusterRole if Rules is empty.
	// Existing Role/ClusterRoles can be only referenced but will never be deleted or updated by Juju.
	// Role/ClusterRoles are created by Juju will be properly labeled.
	Rules []rbacv1.PolicyRule `yaml:"rules"`
}

// Validate returns an error if the spec is not valid.
func (r RoleSpec) Validate() error {
	for _, v := range RoleTypes {
		if r.Type == v {
			return nil
		}
	}
	return errors.NotSupportedf("%q", r.Type)
}

// RoleBindingSpec defines config for referencing to or creating a RoleBinding or ClusterRoleBinding resource.
type RoleBindingSpec struct {
	Name string   `yaml:"name"`
	Type RBACType `yaml:"type"`
}

// Validate returns an error if the spec is not valid.
func (rb RoleBindingSpec) Validate() error {
	for _, v := range RoleBindingTypes {
		if rb.Type == v {
			return nil
		}
	}
	return errors.NotSupportedf("%q", rb.Type)
}

// Capabilities defines RBAC related config.
type Capabilities struct {
	Role        *RoleSpec        `yaml:"role"`
	RoleBinding *RoleBindingSpec `yaml:"roleBinding"`
}

// Validate returns an error if the spec is not valid.
func (c Capabilities) Validate() error {
	if c.Role == nil {
		return errors.New("role is required for capabilities")
	}
	if c.RoleBinding == nil {
		return errors.New("roleBinding is required for capabilities")
	}
	if err := c.Role.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := c.RoleBinding.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ServiceAccountSpec defines spec for referencing to or creating a service account.
type ServiceAccountSpec struct {
	Name                         string        `yaml:"name"`
	AutomountServiceAccountToken *bool         `yaml:"automountServiceAccountToken,omitempty"`
	Capabilities                 *Capabilities `yaml:"capabilities"`
}

// Validate returns an error if the spec is not valid.
func (sa ServiceAccountSpec) Validate() error {
	if sa.Capabilities == nil {
		return errors.New("capabilities is required for service account")
	}
	if err := sa.Capabilities.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// PodSpec defines the data values used to configure
// a pod on the CAAS substrate.
type PodSpec struct {
	OmitServiceFrontend bool `yaml:"omitServiceFrontend"`

	Containers                []ContainerSpec                                              `yaml:"-"`
	InitContainers            []ContainerSpec                                              `yaml:"-"`
	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `yaml:"-"`
	ServiceAccount            *ServiceAccountSpec                                          `yaml:"-"`

	// ProviderPod defines config which is specific to a substrate, eg k8s
	ProviderPod `yaml:"-"`
}

// ProviderPod defines a provider specific pod.
type ProviderPod interface {
	Validate() error
}

// Validate returns an error if the spec is not valid.
func (spec *PodSpec) Validate() error {
	for _, c := range spec.Containers {
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	for _, c := range spec.InitContainers {
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	if spec.ProviderPod != nil {
		return spec.ProviderPod.Validate()
	}
	if spec.ServiceAccount != nil {
		return spec.ServiceAccount.Validate()
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
