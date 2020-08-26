// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/charm/v7"
	"github.com/juju/version"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/storage"
)

// Application broker interface
type Application interface {
	Ensure(config ApplicationConfig) error
	Exists() (DeploymentState, error)
	Delete() error
	Watch() (watcher.NotifyWatcher, error)
	WatchReplicas() (watcher.NotifyWatcher, error)
	State() (ApplicationState, error)
}

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
	// ControllerAddresses is a comma seperated list of controller addresses.
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
