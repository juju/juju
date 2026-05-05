// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	domainmachine "github.com/juju/juju/domain/machine"
	domainnetwork "github.com/juju/juju/domain/network"
	domainprovisioner "github.com/juju/juju/domain/provisioner"
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

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// ShouldKeepInstance reports whether a machine, when removed from Juju,
	// should cause the corresponding cloud instance to be stopped.
	ShouldKeepInstance(ctx context.Context, machineName coremachine.Name) (bool, error)

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

	// AvailabilityZone returns the availability zone for the specified machine.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] if the machine
	// does not exist in the model.
	// - [github.com/juju/juju/domain/machine/errors.AvailabilityZoneNotFound] when
	// no availability zone has been set for the machine.
	AvailabilityZone(ctx context.Context, machineUUID coremachine.UUID) (string, error)

	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(ctx context.Context, mUUID coremachine.UUID) (instance.Id, error)

	// GetSupportedContainersTypes returns the supported container types for the
	// provider.
	GetSupportedContainersTypes(ctx context.Context, mUUID coremachine.UUID) ([]instance.ContainerType, error)

	// GetMachinePrincipalApplications returns the names of the principal
	// (non-subordinate) units for the specified machine.
	GetMachinePrincipalApplications(ctx context.Context, mName coremachine.Name) ([]string, error)

	// WatchMachineContainerLife returns a watcher that observes machine container
	// life changes.
	WatchMachineContainerLife(ctx context.Context, parentMachineName coremachine.Name) (watcher.StringsWatcher, error)

	// GetMachineProvisioningInfo returns the base, placement directive and
	// constraints for the given machine.
	GetMachineProvisioningInfo(ctx context.Context, mName coremachine.Name) (domainmachine.ProvisioningInfo, error)
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
	// GetUnitNamesWithPrincipalOnMachine returns a slice of the unit names and their principals on the given machine.
	GetUnitNamesWithPrincipalOnMachine(ctx context.Context, name coremachine.Name) ([]unit.NameWithPrincipal, error)

	// GetMachinesForApplication returns the names of the machines which have a unit.
	// of the specified application deployed to it.
	GetMachinesForApplication(ctx context.Context, appName string) ([]coremachine.Name, error)
}

// RemovalService provides access to the removal service.
type RemovalService interface {
	// MarkMachineAsDead marks the machine as dead. It will not remove the machine as
	// that is a separate operation. This will advance the machine's life to dead
	// and will not allow it to be transitioned back to alive.
	// Returns an error if the machine does not exist.
	MarkMachineAsDead(context.Context, coremachine.UUID) error

	// MarkInstanceAsDead marks the machine's cloud instance as dead. It will not
	// remove the instance as that is a separate operation. This will advance the
	// instance's life to dead and will not allow it to be transitioned back to
	// alive.
	MarkInstanceAsDead(context.Context, coremachine.UUID) error
}

// ProvisioningService provides access to consolidated provisioning info
// for a machine. This replaces the multiple per-machine service calls with
// a single domain-level aggregation.
type ProvisioningService interface {
	// GetProvisioningInfo returns the complete provisioning information for a
	// machine, consolidating all data from the model and controller databases
	// into a single call.
	GetProvisioningInfo(ctx context.Context, machineName coremachine.Name, isControllerModel bool) (domainprovisioner.ProvisioningInfo, error)
}
