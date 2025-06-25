// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	domainnetwork "github.com/juju/juju/domain/network"
)

// ControllerConfigService is an interface that provides access to the
// controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)

	// SetProviderNetConfig updates the network configuration for a machine using its unique identifier and new interface data.
	SetProviderNetConfig(ctx context.Context, machineUUID machine.UUID, incoming []domainnetwork.NetInterface) error
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// EnsureDeadMachine sets the provided machine's life status to Dead.
	// No error is returned if the provided machine doesn't exist, just nothing
	// gets updated.
	EnsureDeadMachine(context.Context, machine.Name) error

	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(context.Context, machine.Name) (machine.UUID, error)

	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(context.Context, machine.UUID) (instance.Id, error)

	// GetInstanceIDAndName returns the cloud specific instance ID and display
	// name for this machine.
	GetInstanceIDAndName(context.Context, machine.UUID) (instance.Id, string, error)

	// GetHardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	GetHardwareCharacteristics(context.Context, machine.UUID) (*instance.HardwareCharacteristics, error)

	// IsMachineManuallyProvisioned returns whether the machine is a manual
	// machine.
	IsMachineManuallyProvisioned(context.Context, machine.Name) (bool, error)

	// GetMachineLife returns the lifecycle of the machine.
	GetMachineLife(context.Context, machine.Name) (life.Value, error)
}

// StatusService defines the methods that the facade assumes from the Status
// service.
type StatusService interface {
	// GetInstanceStatus returns the cloud specific instance status for this
	// machine.
	GetInstanceStatus(context.Context, machine.Name) (status.StatusInfo, error)

	// SetInstanceStatus sets the cloud specific instance status for this machine.
	SetInstanceStatus(context.Context, machine.Name, status.StatusInfo) error

	// GetMachineStatus returns the status of the specified machine.
	GetMachineStatus(context.Context, machine.Name) (status.StatusInfo, error)

	// SetMachineStatus sets the status of the specified machine.
	SetMachineStatus(context.Context, machine.Name, status.StatusInfo) error
}

// ApplicationService defines the methods that the facade assumes from the Application
// service.
type ApplicationService interface {
	// GetUnitLife returns the lifecycle of the unit.
	GetUnitLife(ctx context.Context, unitName unit.Name) (life.Value, error)
	// GetApplicationLifeByName looks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLifeByName(ctx context.Context, appName string) (life.Value, error)
}
