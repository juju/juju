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
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/environs/config"
	internalcharm "github.com/juju/juju/internal/charm"
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
	GetModelInfo(context.Context) (model.ModelInfo, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
	// the corresponding cloud instance to be stopped.
	ShouldKeepInstance(ctx context.Context, machineName coremachine.Name) (bool, error)
	// SetKeepInstance sets whether the machine cloud instance will be retained
	// when the machine is removed from Juju. This is only relevant if an instance
	// exists.
	SetKeepInstance(ctx context.Context, machineName coremachine.Name, keep bool) error
	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(ctx context.Context, machineUUID coremachine.UUID, instanceID instance.Id, displayName string, hardwareCharacteristics *instance.HardwareCharacteristics) error
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name coremachine.Name) (coremachine.UUID, error)
	// SetAppliedLXDProfileNames sets the list of LXD profile names to the
	// lxd_profile table for the given machine. This method will overwrite the list
	// of profiles for the given machine without any checks.
	SetAppliedLXDProfileNames(ctx context.Context, mUUID coremachine.UUID, profileNames []string) error
	// HardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	HardwareCharacteristics(ctx context.Context, machineUUID coremachine.UUID) (*instance.HardwareCharacteristics, error)
	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(ctx context.Context, mUUID coremachine.UUID) (instance.Id, error)
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
	SpaceByName(ctx context.Context, name network.SpaceName) (*network.SpaceInfo, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
}

// KeyUpdaterService provides access to authorised keys in a model.
type KeyUpdaterService interface {
	// GetInitialAuthorisedKeysForContainer returns the authorised keys to be used
	// when provisioning a new container.
	GetInitialAuthorisedKeysForContainer(ctx context.Context) ([]string, error)
}

// ApplicationService instances implement an application service.
type ApplicationService interface {
	// GetCharmLocatorByApplicationName returns a CharmLocator by application name.
	// It returns an error if the charm can not be found by the name. This can also
	// be used as a cheap way to see if a charm exists without needing to load the
	// charm metadata.
	GetCharmLocatorByApplicationName(ctx context.Context, name string) (charm.CharmLocator, error)

	// GetCharmLXDProfile returns the LXD profile along with the revision of the
	// charm using the charm name, source and revision.
	GetCharmLXDProfile(ctx context.Context, locator charm.CharmLocator) (internalcharm.LXDProfile, charm.Revision, error)
}

// CloudImageMetadataService manages cloud image metadata for provisionning
type CloudImageMetadataService interface {

	// SaveMetadata saves the provided cloud image metadata to the storage and returns an error if the operation fails.
	SaveMetadata(ctx context.Context, metadata []cloudimagemetadata.Metadata) error

	// FindMetadata searches for cloud image metadata based on the given filter criteria in a specific context.
	// It returns a set of image metadata grouped by region
	FindMetadata(ctx context.Context, criteria cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error)
}
