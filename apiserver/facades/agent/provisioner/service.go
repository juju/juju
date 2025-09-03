// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/cloudimagemetadata"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	internalcharm "github.com/juju/juju/internal/charm"
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

	// GetRegionCloudSpec returns a CloudSpec representing the cloud deployment of
	// this model if supported by the provider. If not, an empty structure is
	// returned with no error.
	GetRegionCloudSpec(ctx context.Context) (simplestreams.CloudSpec, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// ShouldKeepInstance reports whether a machine, when removed from Juju,
	// should cause the corresponding cloud instance to be stopped.
	ShouldKeepInstance(ctx context.Context, machineName coremachine.Name) (bool, error)

	// SetKeepInstance sets whether the machine cloud instance will be retained
	// when the machine is removed from Juju. This is only relevant if an
	// instance exists.
	SetKeepInstance(ctx context.Context, machineName coremachine.Name, keep bool) error

	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags.
	SetMachineCloudInstance(
		ctx context.Context,
		machineUUID coremachine.UUID,
		instanceID instance.Id,
		displayName, nonce string,
		hardwareCharacteristics *instance.HardwareCharacteristics,
	) error

	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name coremachine.Name) (coremachine.UUID, error)

	// GetMachineLife returns the GetMachineLife status of the specified machine.
	// It returns a NotFound if the given machine doesn't exist.
	GetMachineLife(context.Context, coremachine.Name) (life.Value, error)

	// AllMachineNames returns the names of all machines in the model.
	AllMachineNames(context.Context) ([]coremachine.Name, error)

	// SetAppliedLXDProfileNames sets the list of LXD profile names to the
	// lxd_profile table for the given machine. This method will overwrite the
	// list of profiles for the given machine without any checks.
	SetAppliedLXDProfileNames(ctx context.Context, mUUID coremachine.UUID, profileNames []string) error

	// GetHardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	GetHardwareCharacteristics(ctx context.Context, machineUUID coremachine.UUID) (*instance.HardwareCharacteristics, error)

	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(ctx context.Context, mUUID coremachine.UUID) (instance.Id, error)

	// GetSupportedContainersTypes returns the supported container types for the
	// provider.
	GetSupportedContainersTypes(ctx context.Context, mUUID coremachine.UUID) ([]instance.ContainerType, error)

	// IsMachineController returns whether the machine is a controller machine.
	// It returns a NotFound if the given machine doesn't exist.
	IsMachineController(ctx context.Context, machineName coremachine.Name) (bool, error)

	// GetMachinePrincipalApplications returns the names of the principal
	// (non-subordinate) units for the specified machine.
	GetMachinePrincipalApplications(ctx context.Context, mName coremachine.Name) ([]string, error)

	// WatchMachineContainerLife returns a watcher that observes machine container
	// life changes.
	WatchMachineContainerLife(ctx context.Context, parentMachineName coremachine.Name) (watcher.StringsWatcher, error)

	// GetMachinePlacementDirective returns the placement structure as it was
	// recorded for the given machine.
	GetMachinePlacementDirective(ctx context.Context, mName coremachine.Name) (*string, error)

	// GetMachineConstraints returns the constraints for the given machine.
	// Empty constraints are returned if no constraints exist for the given
	// machine.
	GetMachineConstraints(ctx context.Context, mName coremachine.Name) (constraints.Value, error)

	// GetMachineBase returns the base for the given machine.
	GetMachineBase(ctx context.Context, mName coremachine.Name) (base.Base, error)

	// UpdateLXDProfiles writes LXD Profiles to LXC for applications on the
	// given machine if the providers supports it. A slice of profile names
	// is returned. If the provider does not support LXDProfiles, no error
	// is returned.
	UpdateLXDProfiles(ctx context.Context, modelName, modelUUID, machineID string) ([]string, error)

	// GetBootstrapEnviron returns the bootstrap environ.
	GetBootstrapEnviron(ctx context.Context) (environs.BootstrapEnviron, error)
}

// StatusService defines the methods that the facade assumes from the Status
// service.
type StatusService interface {
	// GetInstanceStatus returns the cloud specific instance status for this
	// machine.
	GetInstanceStatus(context.Context, coremachine.Name) (status.StatusInfo, error)

	// SetInstanceStatus sets the cloud specific instance status for this machine.
	SetInstanceStatus(context.Context, coremachine.Name, status.StatusInfo) error

	// GetMachineStatus returns the status of the specified machine.
	GetMachineStatus(context.Context, coremachine.Name) (status.StatusInfo, error)

	// SetMachineStatus sets the status of the specified machine.
	SetMachineStatus(context.Context, coremachine.Name, status.StatusInfo) error
}

// StoragePoolGetter instances get a storage pool by name.
type StoragePoolGetter interface {
	// GetStoragePoolByName returns the storage pool with the specified name.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error)
}

// NetworkService provides functionality for working with the network topology,
// setting machine network configuration, and determining container devices
// and addresses.
type NetworkService interface {
	// AllocateContainerAddresses allocates a static address for each of the
	// container NICs in preparedInfo, hosted by the hostInstanceID, if the
	// provider supports it. Returns the network config including all allocated
	// addresses on success.
	// Returns [networkerrors.ContainerAddressesNotSupported] if the provider
	// does not support container addressing.
	AllocateContainerAddresses(ctx context.Context,
		hostInstanceID instance.Id,
		containerName string,
		preparedInfo network.InterfaceInfos,
	) (network.InterfaceInfos, error)

	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)

	// SpaceByName returns a space from state that matches the input name.
	// An error is returned that satisfies errors.NotFound if there is no
	// such space.
	SpaceByName(ctx context.Context, name network.SpaceName) (*network.SpaceInfo, error)

	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)

	// SetMachineNetConfig updates the detected network configuration for
	// the machine with the input UUID.
	SetMachineNetConfig(ctx context.Context, mUUID coremachine.UUID, nics []domainnetwork.NetInterface) error

	// DevicesToBridge accepts the UUID of a host machine and a guest
	// container/VM.
	// It returns the information needed for creating network bridges that
	// will be parents of the guest's virtual network devices.
	// This determination is made based on the guest's space constraints,
	// bindings of applications to run on the guest, and any host bridges
	// that already exist.
	DevicesToBridge(ctx context.Context, hostUUID, guestUUID coremachine.UUID) ([]domainnetwork.DeviceToBridge, error)

	// DevicesForGuest returns the network devices that should be configured
	// in the guest machine with the input UUID, based on the host machine's
	// bridges.
	DevicesForGuest(ctx context.Context, hostUUID, guestUUID coremachine.UUID) ([]domainnetwork.NetInterface, error)
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
	GetCharmLXDProfile(context.Context, charm.CharmLocator) (internalcharm.LXDProfile, charm.Revision, error)

	// GetUnitNamesOnMachine returns a slice of the unit names on the given machine.
	GetUnitNamesOnMachine(context.Context, coremachine.Name) ([]unit.Name, error)

	// GetUnitPrincipal gets the subordinates principal unit. If no principal unit
	// is found, for example, when the unit is not a subordinate, then false is
	// returned.
	GetUnitPrincipal(context.Context, unit.Name) (unit.Name, bool, error)

	// GetApplicationEndpointBindings returns the mapping for each endpoint name and
	// the space ID it is bound to (or empty if unspecified). When no bindings are
	// stored for the application, defaults are returned.
	GetApplicationEndpointBindings(ctx context.Context, appName string) (map[string]network.SpaceUUID, error)

	// GetMachinesForApplication returns the names of the machines which have a unit.
	// of the specified application deployed to it.
	GetMachinesForApplication(ctx context.Context, appName string) ([]coremachine.Name, error)
}

// RemovalService provides access to the removal service.
type RemovalService interface {
	// MarkMachineAsDead marks the machine as dead. It will not remove the machine as
	// that is a separate operation. This will advance the machines's life to dead
	// and will not allow it to be transitioned back to alive.
	// Returns an error if the machine does not exist.
	MarkMachineAsDead(context.Context, coremachine.UUID) error

	// DeleteMachine attempts to delete the specified machine from state entirely.
	DeleteMachine(context.Context, coremachine.UUID) error

	// MarkInstanceAsDead marks the machine's cloud instance as dead. It will not
	// remove the instance as that is a separate operation. This will advance the
	// instance's life to dead and will not allow it to be transitioned back to
	// alive.
	MarkInstanceAsDead(context.Context, coremachine.UUID) error
}

// CloudImageMetadataService manages cloud image metadata for provisionning
type CloudImageMetadataService interface {

	// SaveMetadata saves the provided cloud image metadata to the storage and returns an error if the operation fails.
	SaveMetadata(ctx context.Context, metadata []cloudimagemetadata.Metadata) error

	// FindMetadata searches for cloud image metadata based on the given filter criteria in a specific context.
	// It returns a set of image metadata grouped by region
	FindMetadata(ctx context.Context, criteria cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}
