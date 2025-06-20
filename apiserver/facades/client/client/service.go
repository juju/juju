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
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/port"
	domainrelation "github.com/juju/juju/domain/relation"
	statusservice "github.com/juju/juju/domain/status/service"
)

// ApplicationService defines the methods that the facade assumes from the
// Application service.
type ApplicationService interface {
	// GetUnitUUID returns the UUID for the named unit
	GetUnitUUID(context.Context, unit.Name) (unit.UUID, error)

	// GetLatestPendingCharmhubCharm returns the latest charm that is pending
	// from the charmhub store. If there are no charms, returns is not found, as
	// [applicationerrors.CharmNotFound]. If there are multiple charms, then the
	// latest created at date is returned first.
	GetLatestPendingCharmhubCharm(ctx context.Context, name string, arch architecture.Architecture) (charm.CharmLocator, error)

	// GetExposedEndpoints returns map where keys are endpoint names (or the ""
	// value which represents all endpoints) and values are ExposedEndpoint
	// instances that specify which sources (spaces or CIDRs) can access the
	// opened ports for each endpoint once the application is exposed.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetExposedEndpoints(ctx context.Context, appName string) (map[string]application.ExposedEndpoint, error)

	// GetAllEndpointBindings returns the all endpoint bindings for the model, where
	// endpoints are indexed by the application name for the application which they
	// belong to.
	GetAllEndpointBindings(ctx context.Context) (map[string]map[string]network.SpaceName, error)
}

// StatusService defines the methods that the facade assumes from the Status
// service.
type StatusService interface {
	// GetAllRelationStatuses returns all the relation statuses of the given model.
	GetAllRelationStatuses(ctx context.Context) (map[relation.UUID]status.StatusInfo, error)

	// GetApplicationAndUnitStatuses returns the application statuses of all the
	// applications in the model, indexed by application name.
	GetApplicationAndUnitStatuses(ctx context.Context) (map[string]statusservice.Application, error)

	// GetStatusHistory returns the status history based on the request.
	GetStatusHistory(ctx context.Context, request statusservice.StatusHistoryRequest) ([]status.DetailedStatus, error)

	// GetModelStatus returns the current status of the model.
	GetModelStatus(ctx context.Context) (status.StatusInfo, error)
}

// BlockDeviceService instances can fetch block devices for a machine.
type BlockDeviceService interface {
	// BlockDevices returns the block devices for a machine.
	BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)
	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(ctx context.Context, machineUUID machine.UUID) (instance.Id, error)
	// GetInstanceIDAndName returns the cloud specific instance ID and display
	// name for this machine.
	GetInstanceIDAndName(ctx context.Context, machineUUID machine.UUID) (instance.Id, string, error)
	// GetHardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	GetHardwareCharacteristics(ctx context.Context, machineUUID machine.UUID) (*instance.HardwareCharacteristics, error)
	// AppliedLXDProfiles returns the names of the LXD profiles on the machine.
	AppliedLXDProfileNames(ctx context.Context, machineUUID machine.UUID) ([]string, error)
}

// ModelInfoService provides access to information about the model.
type ModelInfoService interface {
	// GetModelInfo returns information about the current model.
	GetModelInfo(context.Context) (model.ModelInfo, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
	// GetAllDevicesByMachineNames retrieves a mapping of machine names to their
	// associated network interfaces in the model.
	GetAllDevicesByMachineNames(ctx context.Context) (map[machine.Name][]domainnetwork.NetInterface, error)
}

// PortService defines the methods that the facade assumes from the Port
// service.
type PortService interface {
	// GetAllOpenedPorts returns the opened ports in the model, grouped by unit
	// name.
	GetAllOpenedPorts(ctx context.Context) (port.UnitGroupedPortRanges, error)

	// GetUnitOpenedPorts returns the opened ports for a given unit uuid,
	// grouped by endpoint.
	GetUnitOpenedPorts(context.Context, unit.UUID) (network.GroupedPortRanges, error)
}

// RelationService provides methods to interact with and retrieve details of
// relations within a model.
type RelationService interface {
	// GetAllRelationDetails return all uuid of all relation for the current model.
	GetAllRelationDetails(ctx context.Context) ([]domainrelation.RelationDetailsResult, error)
}
