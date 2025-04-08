// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	domainapplication "github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	internalcharm "github.com/juju/juju/internal/charm"
)

// Services represents all the services that the uniter facade requires.
type Services struct {
	ApplicationService      ApplicationService
	StatusService           StatusService
	ControllerConfigService ControllerConfigService
	MachineService          MachineService
	ModelConfigService      ModelConfigService
	ModelInfoService        ModelInfoService
	NetworkService          NetworkService
	PortService             PortService
	RelationService         RelationService
	SecretService           SecretService
	UnitStateService        UnitStateService
	StubService             StubService
}

// ControllerConfigService provides the controller configuration for the model.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelConfigService is used by the provisioner facade to get model config.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch() (watcher.StringsWatcher, error)
}

// ModelInfoService describes the service for interacting and reading the
// underlying model information.
type ModelInfoService interface {
	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(context.Context) (model.ModelInfo, error)

	// CloudAPIVersion returns the cloud API version for the model's cloud.
	CloudAPIVersion(context.Context) (string, error)
}

// StubService will be replaced once the implementation is finished.
type StubService interface {
	// CloudSpec returns the cloud spec for the model.
	CloudSpec(ctx context.Context) (cloudspec.CloudSpec, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// GetApplicationLife looks up the life of the specified application.
	GetApplicationLife(ctx context.Context, unitName string) (life.Value, error)

	// GetUnitLife looks up the life of the specified unit.
	GetUnitLife(ctx context.Context, unitName coreunit.Name) (life.Value, error)

	// GetUnitUUID returns the UUID for the named unit.
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)

	// EnsureUnitDead is called by the unit agent just before it terminates.
	EnsureUnitDead(ctx context.Context, unitName coreunit.Name, leadershipRevoker leadership.Revoker) error

	// DeleteUnit deletes the specified unit.
	DeleteUnit(ctx context.Context, unitName coreunit.Name) error

	// DestroyUnit prepares a unit for removal from the model.
	DestroyUnit(ctx context.Context, unitName coreunit.Name) error

	// WatchApplication returns a NotifyWatcher for changes to the application.
	WatchApplication(ctx context.Context, name string) (watcher.NotifyWatcher, error)

	// GetApplicationIDByUnitName returns the application ID for the named unit.
	//
	// Returns [applicationerrors.UnitNotFound] if the unit is not found.
	GetApplicationIDByUnitName(ctx context.Context, unitName coreunit.Name) (coreapplication.ID, error)

	// GetApplicationIDByName returns an application ID by application name.
	//
	// Returns [applicationerrors.ApplicationNotFound] if the application is not
	// found.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// GetCharmModifiedVersion looks up the charm modified version of the given
	// application.
	GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error)

	// GetAvailableCharmArchiveSHA256 returns the SHA256 hash of the charm
	// archive for the given charm name, source and revision. If the charm is
	// not available, [applicationerrors.CharmNotResolved] is returned.
	GetAvailableCharmArchiveSHA256(ctx context.Context, locator charm.CharmLocator) (string, error)

	// GetCharmLXDProfile returns the LXD profile along with the revision of the
	// charm using the charm name, source and revision.
	GetCharmLXDProfile(ctx context.Context, locator charm.CharmLocator) (internalcharm.LXDProfile, charm.Revision, error)

	// GetApplicationConfig returns the application config attributes for the
	// configuration.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationConfig(ctx context.Context, uuid coreapplication.ID) (coreconfig.ConfigAttributes, error)

	// GetUnitRefreshAttributes returns the refresh attributes for the unit.
	GetUnitRefreshAttributes(ctx context.Context, unitName coreunit.Name) (domainapplication.UnitAttributes, error)
}

// StatusService describes the ability to retrieve and persist
// application statuses
type StatusService interface {
	// SetApplicationStatusForUnitLeader sets the application status using the
	// leader unit of the application.
	SetApplicationStatusForUnitLeader(context.Context, coreunit.Name, corestatus.StatusInfo) error

	// GetUnitWorkloadStatus returns the workload status of the specified unit
	GetUnitWorkloadStatus(context.Context, coreunit.Name) (corestatus.StatusInfo, error)

	// SetUnitWorkloadStatus sets the workload status of the specified unit
	SetUnitWorkloadStatus(context.Context, coreunit.Name, corestatus.StatusInfo) error

	// SetUnitAgentStatus sets the agent status of the specified unit.
	SetUnitAgentStatus(context.Context, coreunit.Name, corestatus.StatusInfo) error

	// GetApplicationAndUnitStatusesForUnitWithLeader returns the display status
	// of the application the specified unit belongs to, and the workload statuses
	// of all the units that belong to that application, indexed by unit name.
	GetApplicationAndUnitStatusesForUnitWithLeader(
		context.Context,
		coreunit.Name,
	) (
		corestatus.StatusInfo,
		map[coreunit.Name]corestatus.StatusInfo,
		error,
	)

	// GetUnitWorkloadStatusesForApplication returns the workload statuses of
	// all units in the specified application, indexed by unit name
	GetUnitWorkloadStatusesForApplication(ctx context.Context, appID coreapplication.ID) (map[coreunit.Name]corestatus.StatusInfo, error)

	// SetRelationStatus sets the status of the relation to the status provided.
	// Status may only be set by the application leader.
	SetRelationStatus(
		ctx context.Context,
		unitName coreunit.Name,
		relationUUID corerelation.UUID,
		info corestatus.StatusInfo,
	) error
}

// UnitStateService describes the ability to retrieve and persist
// unit agent state for informing hook reconciliation.
type UnitStateService interface {
	// SetState persists the input unit state.
	SetState(context.Context, unitstate.UnitState) error
	// GetState returns the full unit state. The state may be empty.
	GetState(ctx context.Context, uuid coreunit.Name) (unitstate.RetrievedUnitState, error)
}

// PortService describes the ability to open and close port ranges for units.
type PortService interface {
	// UpdateUnitPorts opens and closes ports for the endpoints of a given unit.
	UpdateUnitPorts(ctx context.Context, unitUUID coreunit.UUID, openPorts, closePorts network.GroupedPortRanges) error

	// GetMachineOpenedPorts returns the opened ports for all the units on the
	// machine. Opened ports are grouped first by unit name and then by endpoint.
	GetMachineOpenedPorts(ctx context.Context, machineUUID string) (map[coreunit.Name]network.GroupedPortRanges, error)

	// GetUnitOpenedPorts returns the opened ports for a given unit uuid, grouped
	// by endpoint.
	GetUnitOpenedPorts(ctx context.Context, unitUUID coreunit.UUID) (network.GroupedPortRanges, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// SpaceByName returns a space from state that matches the input name.
	// An error is returned that satisfied errors.NotFound if the space was not found
	// or an error static any problems fetching the given space.
	SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
	RequireMachineReboot(ctx context.Context, uuid string) error

	// ClearMachineReboot removes the reboot flag of the machine referenced by its UUID if a reboot has previously been required.
	ClearMachineReboot(ctx context.Context, uuid string) error

	// IsMachineRebootRequired checks if the machine referenced by its UUID requires a reboot.
	IsMachineRebootRequired(ctx context.Context, uuid string) (bool, error)

	// ShouldRebootOrShutdown determines whether a machine should reboot or shutdown
	ShouldRebootOrShutdown(ctx context.Context, uuid string) (coremachine.RebootAction, error)

	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns an errors.MachineNotFound if the machine does not exist.
	GetMachineUUID(ctx context.Context, machineName coremachine.Name) (string, error)

	// AppliedLXDProfileNames returns the names of the LXD profiles on the machine.
	AppliedLXDProfileNames(ctx context.Context, mUUID string) ([]string, error)

	// WatchMachineCloudInstances returns a StringsWatcher that is subscribed to
	// the changes in the machine_cloud_instance table in the model.
	WatchLXDProfiles(ctx context.Context, machineUUID string) (watcher.NotifyWatcher, error)

	// AvailabilityZone returns the hardware characteristics of the
	// specified machine.
	AvailabilityZone(ctx context.Context, machineUUID string) (string, error)
}

// RelationService defines the methods that the facade assumes from the
// Relation service.
type RelationService interface {
	// EnterScope indicates that the provided unit has joined the relation.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.PotentialRelationUnitNotValid] if the unit entering
	//     scope is a subordinate and the endpoint scope is charm.ScopeContainer
	//     where the other application is a principal, but not in the current
	//     relation.
	EnterScope(
		ctx context.Context,
		relationID corerelation.UUID,
		unitName coreunit.Name,
	) error

	// GetLocalRelationApplicationSettings returns the application settings
	// for the given application and relation identifier combination.
	// ApplicationSettings may only be read by the application leader.
	// Returns NotFound if this unit is not the leader, if the application or
	// relation is not found.
	GetLocalRelationApplicationSettings(
		ctx context.Context,
		unitName coreunit.Name,
		relationUUID corerelation.UUID,
		applicationID coreapplication.ID,
	) (map[string]string, error)

	// GetRelationDetails returns the relation details requested by the uniter
	// for a relation.
	GetRelationDetails(ctx context.Context, relationUUID corerelation.UUID) (relation.RelationDetails, error)

	// GetRelationUnit returns the relation unit UUID for the given unit within
	// the given relation.
	GetRelationUnit(
		ctx context.Context,
		relationUUID corerelation.UUID,
		unitName coreunit.Name,
	) (corerelation.UnitUUID, error)

	// GetRelationUnitSettings returns the unit settings for the
	// given relation unit identifier.
	GetRelationUnitSettings(
		ctx context.Context,
		relationUnitUUID corerelation.UnitUUID,
	) (map[string]string, error)

	// GetRelationUUIDByKey returns a relation UUID for the given relation Key.
	// The relation key is a ordered space separated string of the endpoint
	// names of the relation.
	// The following error types can be expected:
	// - [relationerrors.RelationNotFound]: when no relation exists for the given key.
	GetRelationUUIDByKey(ctx context.Context, relationKey corerelation.Key) (corerelation.UUID, error)

	// GetRelationByID returns the relation uuid based on the relation ID.
	GetRelationUUIDByID(ctx context.Context, relationID int) (corerelation.UUID, error)

	// GetRelationsStatusesForUnit returns RelationUnitStatus for
	// any relation the unit is part of.
	GetRelationsStatusForUnit(ctx context.Context, unitUUID coreunit.UUID) ([]relation.RelationUnitStatus, error)

	// GetRemoteRelationApplicationSettings returns the application settings
	// for the given application and relation identifier combination.
	// Returns NotFound if the application or relation is not found.
	GetRemoteRelationApplicationSettings(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID coreapplication.ID,
	) (map[string]string, error)

	// LeaveScope updates the given relation to indicate it is not in scope.
	LeaveScope(ctx context.Context, relationID corerelation.UnitUUID) error

	// UpdateRelationApplicationSettings updates settings for a specific application
	// relation combination.
	UpdateRelationApplicationSettings(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID coreapplication.ID,
		settings map[string]string,
	) error

	// UpdateRelationUnitSettings updates settings for a specific unit
	// relation combination.
	UpdateRelationUnitSettings(
		ctx context.Context,
		relationUnitUUID corerelation.UnitUUID,
		settings map[string]string,
	) error

	// WatchLifeSuspendedStatus returns a watcher that notifies of changes to
	// the life or suspended status any relation the unit's application is part
	// of. If the unit is a subordinate, its principal application is watched.
	WatchLifeSuspendedStatus(
		ctx context.Context,
		unitID coreunit.UUID,
	) (watcher.StringsWatcher, error)
}
