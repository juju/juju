// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/charm/v8"
	"github.com/juju/version"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
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

	// IntroductionSecret
	IntroductionSecret string
	// ControllerAddresses is a comma separated list of controller addresses.
	// TODO: Use model-operator service instead for introduction, so controller addresses can change
	// without having to update deployed application.
	ControllerAddresses string
	// ControllerCertBundle is a PEM cert bundle for talking to the Juju controller.
	ControllerCertBundle string

	// Charm of the Application.
	Charm charm.Charm

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
