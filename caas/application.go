// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/version"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/storage"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/application_mock.go github.com/juju/juju/caas Application

// Application is for interacting with the CAAS substrate.
type Application interface {
	Ensure(config ApplicationConfig) error
	Exists() (DeploymentState, error)
	Delete() error
	Watch() (watcher.NotifyWatcher, error)
	WatchReplicas() (watcher.NotifyWatcher, error)
	State() (ApplicationState, error)
	Units() ([]Unit, error)

	ServiceInterface
}

// ServicePort represents service ports mapping from service to units.
type ServicePort struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	TargetPort int    `json:"target-port"`
	Protocol   string `json:"protocol"`
}

// ServiceParam defines parameters for an UpdateService request.
type ServiceParam struct {
	Type  string        `json:"type"`
	Ports []ServicePort `json:"ports"`
}

// ServiceInterface provides the API to get/set service.
type ServiceInterface interface {
	// UpdateService updates the default service with specific service type and port mappings.
	UpdateService(ServiceParam) error

	UpdatePorts(ports []ServicePort, updateContainerPorts bool) error
}

// ApplicationState represents the application state.
type ApplicationState struct {
	DesiredReplicas int
	Replicas        []string
}

// ApplicationConfig is the config passed to the application units.
type ApplicationConfig struct {
	// AgentVersion is the Juju version of the agent image.
	AgentVersion version.Number
	// AgentImagePath is the docker registry URL for the image.
	AgentImagePath string

	// CharmModifiedVersion is a monotonically incrementing version number
	// that represents the version of the charm and resources with regards to
	// this application. The CAAS provider will pass this to the uniter worker
	// to ensure the container infrastructure matches the charm.
	CharmModifiedVersion int

	// CharmBaseImage is the docker image used by the charm.
	CharmBaseImage resources.DockerImageDetails

	// Containers is the list of containers that make up the container (excluding uniter and init containers).
	Containers map[string]ContainerConfig

	// IntroductionSecret
	IntroductionSecret string
	// ControllerAddresses is a comma separated list of controller addresses.
	// TODO: Use model-operator service instead for introduction, so controller addresses can change
	// without having to update deployed application.
	ControllerAddresses string
	// ControllerCertBundle is a PEM cert bundle for talking to the Juju controller.
	ControllerCertBundle string

	// ResourceTags is a set of tags to set on the operator pod.
	ResourceTags map[string]string

	// Constraints is a set of constraints on
	// the pod to create.
	Constraints constraints.Value

	// Filesystems is a set of parameters for filesystems that should be created.
	Filesystems []storage.KubernetesFilesystemParams

	// Devices is a set of parameters for Devices that is required.
	Devices []devices.KubernetesDeviceParams
}

// ContainerConfig describes a container that is deployed alonside the uniter/charm container.
type ContainerConfig struct {
	// Name of the container.
	Name string

	// Image used to create the container.
	Image resources.DockerImageDetails

	// Mounts to storage that are to be provided within this container.
	Mounts []MountConfig
}

// MountConfig describes a storage that should be mounted to a container.
type MountConfig struct {
	// StorageName is the name of the storage as specified in the charm.
	StorageName string

	// Path is the mount point inside the container.
	Path string
}
