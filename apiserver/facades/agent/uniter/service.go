// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"time"

	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/instance"
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
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/domain/resolve"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	internalcharm "github.com/juju/juju/internal/charm"
)

// Services represents all the services that the uniter facade requires.
type Services struct {
	ApplicationService      ApplicationService
	ResolveService          ResolveService
	StatusService           StatusService
	ControllerConfigService ControllerConfigService
	ControllerNodeService   ControllerNodeService
	MachineService          MachineService
	ModelConfigService      ModelConfigService
	ModelInfoService        ModelInfoService
	ModelProviderService    ModelProviderService
	PortService             PortService
	NetworkService          NetworkService
	RelationService         RelationService
	SecretService           SecretService
	UnitStateService        UnitStateService
	RemovalService          RemovalService
}

// ControllerConfigService provides the controller configuration for the model.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ControllerNodeService defines the methods on the controller node service
// that are needed by APIAddresser used by the uniter API.
type ControllerNodeService interface {
	// GetAllAPIAddressesForAgents returns a map of controller IDs to their API
	// addresses that are available for agents. The map is keyed by controller
	// ID, and the values are slices of strings representing the API addresses
	// for each controller node.
	GetAllAPIAddressesForAgents(ctx context.Context) (map[string][]string, error)
	// GetAllAPIAddressesForAgentsInPreferredOrder returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgentsInPreferredOrder(ctx context.Context) ([]string, error)
	// WatchControllerAPIAddresses returns a watcher that observes changes to the
	// controller ip addresses.
	WatchControllerAPIAddresses(context.Context) (watcher.NotifyWatcher, error)
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

// ModelProviderService providers access to the model provider service.
type ModelProviderService interface {
	// GetCloudSpec returns the cloud spec for the model.
	GetCloudSpec(ctx context.Context) (cloudspec.CloudSpec, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// GetApplicationLifeByName looks up the life of the specified application.
	GetApplicationLifeByName(ctx context.Context, name string) (life.Value, error)

	// GetUnitLife looks up the life of the specified unit.
	GetUnitLife(ctx context.Context, unitName coreunit.Name) (life.Value, error)

	// GetUnitUUID returns the UUID for the named unit.
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)

	// GetUnitPrincipal returns the unit's principal unit if it exists
	GetUnitPrincipal(ctx context.Context, unitName coreunit.Name) (coreunit.Name, bool, error)

	// GetUnitMachineName gets the name of the unit's machine.
	//
	// The following errors may be returned:
	//   - [applicationerrors.UnitMachineNotAssigned] if the unit does not have a
	//     machine assigned.
	GetUnitMachineName(ctx context.Context, unitName coreunit.Name) (coremachine.Name, error)

	// GetUnitMachineUUID gets the uuid of the unit's machine. If the unit's
	// machine cannot be found [applicationerrors.UnitMachineNotAssigned] is
	// returned.
	GetUnitMachineUUID(ctx context.Context, unitName coreunit.Name) (coremachine.UUID, error)

	// GetUnitNamesForApplication returns a slice of the unit names for the given application
	GetUnitNamesForApplication(ctx context.Context, appName string) ([]coreunit.Name, error)

	// WatchApplication returns a NotifyWatcher for changes to the application.
	WatchApplication(ctx context.Context, name string) (watcher.NotifyWatcher, error)

	// WatchUnitForLegacyUniter watches for some specific changes to the unit with
	// the given name. The watcher will emit a notification when there is a change to
	// the unit's inherent properties, it's subordinates or it's resolved mode.
	//
	// If the unit does not exist an error satisfying [applicationerrors.UnitNotFound]
	// will be returned.
	WatchUnitForLegacyUniter(ctx context.Context, unitName coreunit.Name) (watcher.NotifyWatcher, error)

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

	// AddIAASSubordinateUnit adds a IAAS unit to the specified subordinate
	// application to the application on the same machine as the given principal
	// unit and records the principal-subordinate relationship.
	AddIAASSubordinateUnit(ctx context.Context, subordinateAppID coreapplication.ID, principalUnitName coreunit.Name) error

	// SetUnitWorkloadVersion sets the workload version for the given unit.
	SetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name, version string) error

	// GetUnitWorkloadVersion returns the workload version for the given unit.
	GetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name) (string, error)

	// GetApplicationConfigWithDefaults returns the application config
	// attributes for the configuration, or their charm default if the config
	// attribute is not set.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationConfigWithDefaults(ctx context.Context, appID coreapplication.ID) (coreconfig.ConfigAttributes, error)

	// WatchApplicationConfigHash watches for changes to the specified application's
	// config hash.
	WatchApplicationConfigHash(ctx context.Context, name string) (watcher.StringsWatcher, error)

	// WatchUnitAddressesHash watches for changes to the specified unit's
	// addresses hash, as well as changes to the endpoint bindings for the spaces
	// the addresses belong to.
	//
	// If the unit does not exist an error satisfying [applicationerrors.UnitNotFound]
	// will be returned.
	WatchUnitAddressesHash(ctx context.Context, unitName coreunit.Name) (watcher.StringsWatcher, error)

	// GetUnitSubordinates returns the names of all the subordinate units of the
	// given principal unit.
	GetUnitSubordinates(ctx context.Context, unitName coreunit.Name) ([]coreunit.Name, error)

	// GetCharmLocatorByApplicationName returns a CharmLocator by application name.
	GetCharmLocatorByApplicationName(ctx context.Context, name string) (charm.CharmLocator, error)

	// ShouldAllowCharmUpgradeOnError indicates if the units of an application should
	// upgrade to the latest version of the application charm even if they are in
	// error state.
	ShouldAllowCharmUpgradeOnError(ctx context.Context, appName string) (bool, error)

	// WatchUnitActions watches for all updates to actions for the specified unit,
	// emitting action ids.
	//
	// If the unit does not exist an error satisfying [applicationerrors.UnitNotFound]
	// will be returned.
	WatchUnitActions(ctx context.Context, unitName coreunit.Name) (watcher.StringsWatcher, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetUnitPublicAddress returns the public address for the specified unit.
	// For k8s provider, it will return the first public address of the cloud
	// service if any, the first public address of the cloud container otherwise.
	// For machines provider, it will return the first public address of the
	// machine.
	//
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit does not exist
	// - [network.NoAddressError] if the unit has no public address associated
	GetUnitPublicAddress(ctx context.Context, unitName coreunit.Name) (network.SpaceAddress, error)

	// GetUnitPrivateAddress returns the private address for the specified unit.
	// For k8s provider, it will return the first private address of the cloud
	// service if any, the first private address of the cloud container otherwise.
	// For machines provider, it will return the first private address of the
	// machine.
	//
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit does not exist
	// - [network.NoAddressError] if the unit has no private address associated
	GetUnitPrivateAddress(ctx context.Context, unitName coreunit.Name) (network.SpaceAddress, error)

	// GetUnitRelationNetwork retrieves network relation information for a given
	// unit and relation key.
	//
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit does not exist
	// - [relationerrors.RelationNotFound] if the relation key doesn't belong to
	//   the unit.
	GetUnitRelationNetwork(ctx context.Context, unitName coreunit.Name, relKey corerelation.Key) (domainnetwork.UnitNetwork, error)

	// GetUnitEndpointNetworks retrieves network relation information for a given unit and specified endpoints.
	// It returns exactly one info for each endpoint names passed in argument,
	// but doesn't enforce the order. Each info has an endpoint name that should match
	// one of the endpoint names, one info for each endpoint names.
	//
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit does not exist
	GetUnitEndpointNetworks(ctx context.Context, unitName coreunit.Name, endpointNames []string) ([]domainnetwork.UnitNetwork, error)
}

type ResolveService interface {
	// UnitResolveMode returns the resolve mode for the given unit. If no unit is found
	// with the given name, an error satisfying [resolveerrors.UnitNotFound] is returned.
	// if no resolved marker is found for the unit, an error satisfying
	// [resolveerrors.UnitNotResolved] is returned.
	UnitResolveMode(context.Context, coreunit.Name) (resolve.ResolveMode, error)

	// ClearResolved removes any resolved marker from the unit. If the unit is not
	// found, an error satisfying [resolveerrors.UnitNotFound] is returned.
	ClearResolved(context.Context, coreunit.Name) error

	// WatchUnitResolveMode returns a watcher that emits notification when the resolve
	// mode of the specified unit changes.
	//
	// If the unit does not exist an error satisfying [resolveerrors.UnitNotFound]
	// will be returned.
	WatchUnitResolveMode(context.Context, coreunit.Name) (watcher.NotifyWatcher, error)
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

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// RequireMachineReboot sets the machine referenced by its UUID as requiring
	// a reboot.
	RequireMachineReboot(ctx context.Context, uuid coremachine.UUID) error

	// ClearMachineReboot removes the reboot flag of the machine referenced by
	// its UUID if a reboot has previously been required.
	ClearMachineReboot(ctx context.Context, uuid coremachine.UUID) error

	// IsMachineRebootRequired checks if the machine referenced by its UUID
	// requires a reboot.
	IsMachineRebootRequired(ctx context.Context, uuid coremachine.UUID) (bool, error)

	// ShouldRebootOrShutdown determines whether a machine should reboot or
	// shutdown
	ShouldRebootOrShutdown(ctx context.Context, uuid coremachine.UUID) (coremachine.RebootAction, error)

	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns an errors.MachineNotFound if the machine does not exist.
	GetMachineUUID(ctx context.Context, machineName coremachine.Name) (coremachine.UUID, error)

	// AppliedLXDProfileNames returns the names of the LXD profiles on the
	// machine.
	AppliedLXDProfileNames(ctx context.Context, mUUID coremachine.UUID) ([]string, error)

	// WatchMachineCloudInstances returns a StringsWatcher that is subscribed to
	// the changes in the machine_cloud_instance table in the model.
	WatchLXDProfiles(ctx context.Context, machineUUID coremachine.UUID) (watcher.NotifyWatcher, error)

	// AvailabilityZone returns the hardware characteristics of the specified
	// machine.
	AvailabilityZone(ctx context.Context, machineUUID coremachine.UUID) (string, error)

	// IsMachineManuallyProvisioned returns whether the machine is a manual
	// machine.
	IsMachineManuallyProvisioned(ctx context.Context, machineName coremachine.Name) (bool, error)

	// GetSupportedContainersTypes returns the supported container types for the
	// provider.
	GetSupportedContainersTypes(ctx context.Context, mUUID coremachine.UUID) ([]instance.ContainerType, error)
}

// RelationService defines the methods that the facade assumes from the
// Relation service.
type RelationService interface {
	// EnterScope indicates that the provided unit has joined the relation.
	// When the unit has already entered its relation scope, EnterScope will report
	// success but make no changes to state. The unit's settings are created or
	// overwritten in the relation according to the supplied map.
	//
	// If there is a subordinate application related to the unit entering scope that
	// needs a subordinate unit creating, then the subordinate unit will be created
	// with the provided createSubordinate function.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.PotentialRelationUnitNotValid] if the unit entering
	//     scope is a subordinate and the endpoint scope is charm.ScopeContainer
	//     where the other application is a principal, but not in the current
	//     relation.
	EnterScope(
		ctx context.Context,
		relationUUID corerelation.UUID,
		unitName coreunit.Name,
		settings map[string]string,
		createSubordinate relation.SubordinateCreator,
	) error

	// GetGoalStateRelationDataForApplication returns GoalStateRelationData for all
	// relations the given application is in, modulo peer relations.
	//
	// The following error types can be expected to be returned:
	//   - [relationerrors.ApplicationIDNotValid] is returned if the application
	//     UUID is not valid.
	GetGoalStateRelationDataForApplication(
		ctx context.Context,
		applicationID coreapplication.ID,
	) ([]relation.GoalStateRelationData, error)

	// GetRelationApplicationSettingsWithLeader returns the application settings
	// for the given application and relation identifier combination.
	//
	// Only the leader unit may read the settings of the application in the local
	// side of the relation.
	//
	// The following error types can be expected to be returned:
	//   - [corelease.ErrNotHeld] if the unit is not the leader.
	//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
	//     application is not part of the relation.
	//   - [relationerrors.RelationNotFound] is returned if the relation UUID
	//     is not found.
	GetRelationApplicationSettingsWithLeader(
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

	// GetRelationUnitChanges retrieves updates to relation units and
	// applications based on provided UUIDs.
	// It returns a RelationUnitsChange object describing changes and an error
	// if the operation fails.
	GetRelationUnitChanges(
		ctx context.Context,
		unitUUIDs []coreunit.UUID,
		appUUIDs []coreapplication.ID,
	) (relation.RelationUnitsChange, error)

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

	// GetRelationUUIDByID returns the relation uuid based on the relation ID.
	GetRelationUUIDByID(ctx context.Context, relationID int) (corerelation.UUID, error)

	// GetRelationsStatusForUnit returns RelationUnitStatus for
	// any relation the unit is part of.
	GetRelationsStatusForUnit(ctx context.Context, unitUUID coreunit.UUID) ([]relation.RelationUnitStatus, error)

	// GetRelationApplicationSettings returns the application settings
	// for the given application and relation identifier combination.
	//
	// This function does not check leadership, so should only be used to check
	// the settings of applications on the other end of the relation to the caller.
	GetRelationApplicationSettings(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID coreapplication.ID,
	) (map[string]string, error)

	// LeaveScope updates the given relation to indicate it is not in scope.
	LeaveScope(ctx context.Context, relationID corerelation.UnitUUID) error

	// SetRelationApplicationAndUnitSettings records settings for a unit and
	// an application in a relation.
	//
	// The following error types can be expected to be returned:
	//   - [corelease.ErrNotHeld] if the unit is not the leader.
	//   - [relationerrors.RelationUnitNotFound] is returned if the
	//     relation unit is not found.
	SetRelationApplicationAndUnitSettings(
		ctx context.Context,
		unitName coreunit.Name,
		relationUnitUUID corerelation.UnitUUID,
		applicationSettings, unitSettings map[string]string,
	) error

	// WatchLifeSuspendedStatus returns a watcher that notifies of changes to
	// the life or suspended status any relation the unit's application is part
	// of. If the unit is a subordinate, its principal application is watched.
	WatchLifeSuspendedStatus(
		ctx context.Context,
		unitID coreunit.UUID,
	) (watcher.StringsWatcher, error)

	// WatchRelatedUnits returns a watcher that notifies of changes to counterpart units in
	// the relation.
	WatchRelatedUnits(
		ctx context.Context,
		unitName coreunit.Name,
		relationUUID corerelation.UUID,
	) (watcher.StringsWatcher, error)
}

type RemovalService interface {
	// RemoveUnit checks if a unit with the input name exists.
	// If it does, the unit is guaranteed after this call to be:
	//   - No longer alive.
	//   - Removed or scheduled to be removed with the input force qualification.
	//   - If the unit is the last one on the machine, the machine will also
	//     guaranteed to be no longer alive and scheduled for removal.
	//
	// The input wait duration is the time that we will give for the normal
	// life-cycle advancement and removal to finish before forcefully removing the
	// unit. This duration is ignored if the force argument is false.
	// The UUID for the scheduled removal job is returned.
	RemoveUnit(
		ctx context.Context,
		unitUUID coreunit.UUID,
		force bool,
		wait time.Duration,
	) (removal.UUID, error)

	// MarkUnitAsDead marks the unit as dead. It will not remove the unit as
	// that is a separate operation. This will advance the unit's life to dead
	// and will not allow it to be transitioned back to alive.
	MarkUnitAsDead(context.Context, coreunit.UUID) error
}
