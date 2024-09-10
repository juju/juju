// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/storage"
)

// AgentProvisionerService provides access to container config.
type AgentProvisionerService interface {
	// ContainerManagerConfigForType returns the container manager config for
	// the given container type.
	ContainerManagerConfigForType(context.Context, instance.ContainerType) (containermanager.Config, error)
	// ContainerConfig returns the container configuration.
	ContainerConfig(ctx context.Context) (container.Config, error)
	// ContainerNetworkingMethod returns the networking method to use for newly
	// provisioned containers.
	ContainerNetworkingMethod(ctx context.Context) (containermanager.NetworkingMethod, error)
}

// ControllerConfigService is the interface that the provisioner facade
// uses to get the controller config.
type ControllerConfigService interface {
	// ControllerConfig returns this controller's config.
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelConfigService is the interface that the provisioner facade uses to get
// the model config.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ModelInfoService describe the service for interacting and reading the underlying
// model information.
type ModelInfoService interface {
	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(context.Context) (model.ReadOnlyModel, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
	// the corresponding cloud instance to be stopped.
	// It returns a NotFound if the given machine doesn't exist.
	ShouldKeepInstance(ctx context.Context, machineName coremachine.Name) (bool, error)
	// SetKeepInstance sets whether the machine cloud instance will be retained
	// when the machine is removed from Juju. This is only relevant if an instance
	// exists.
	// It returns a NotFound if the given machine doesn't exist.
	SetKeepInstance(ctx context.Context, machineName coremachine.Name, keep bool) error
}

// StoragePoolGetter instances get a storage pool by name.
type StoragePoolGetter interface {
	// GetStoragePoolByName returns the storage pool with the specified name.
	GetStoragePoolByName(ctx context.Context, name string) (*storage.Config, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// SpaceByName returns a space from state that matches the input name.
	// An error is returned that satisfied errors.NotFound if the space was not found
	// or an error static any problems fetching the given space.
	SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
}

// KeyUpdaterService provides access to authorised keys in a model.
type KeyUpdaterService interface {
	// GetInitialAuthorisedKeysForContainer returns the authorised keys to be used
	// when provisioning a new container.
	GetInitialAuthorisedKeysForContainer(ctx context.Context) ([]string, error)
}
