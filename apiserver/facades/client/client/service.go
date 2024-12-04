// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	domainmodel "github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/port"
)

// BlockDeviceService instances can fetch block devices for a machine.
type BlockDeviceService interface {
	BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
}

// ModelInfoService provides access to information about the model.
type ModelInfoService interface {
	// GetModelInfo returns information about the current model.
	GetModelInfo(context.Context) (model.ReadOnlyModel, error)
	// Status returns the current status of the model.
	// The following error types can be expected to be returned:
	// - [modelerrors.NotFound]: When the model does not exist.
	Status(context.Context) (domainmodel.StatusInfo, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (string, error)
	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(ctx context.Context, machineUUID string) (instance.Id, error)
	// InstanceIDAndName returns the cloud specific instance ID and display name for
	// this machine.
	InstanceIDAndName(ctx context.Context, machineUUID string) (instance.Id, string, error)
	// HardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	HardwareCharacteristics(ctx context.Context, machineUUID string) (*instance.HardwareCharacteristics, error)
	// AppliedLXDProfiles returns the names of the LXD profiles on the machine.
	AppliedLXDProfileNames(ctx context.Context, machineUUID string) ([]string, error)
}

// ApplicationService defines the methods that the facade assumes from the
// Application service.
type ApplicationService interface {
	// GetUnitUUID returns the UUID for the named unit
	GetUnitUUID(context.Context, unit.Name) (unit.UUID, error)
}

// PortService defines the methods that the facade assumes from the Port service.
type PortService interface {
	// GetAllOpenedPorts returns the opened ports in the model, grouped by unit
	// name.
	GetAllOpenedPorts(ctx context.Context) (port.UnitGroupedPortRanges, error)

	// GetUnitOpenedPorts returns the opened ports for a given unit uuid, grouped
	// by endpoint.
	GetUnitOpenedPorts(context.Context, unit.UUID) (network.GroupedPortRanges, error)
}
