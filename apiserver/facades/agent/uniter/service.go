// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/environs/config"
	internalcharm "github.com/juju/juju/internal/charm"
)

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
}

// CloudService provides access to clouds.
type CloudService interface {
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
	WatchCloud(ctx context.Context, name string) (watcher.NotifyWatcher, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
	WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error)
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
	// Returns [github.com/juju/juju/domain/application.UnitNotFound] if the
	// unit is not found.
	GetApplicationIDByUnitName(ctx context.Context, unitName coreunit.Name) (coreapplication.ID, error)

	// GetApplicationIDByName returns an application ID by application name.
	//
	// Returns [github.com/juju/juju/domain/application.ApplicationNotFound] if
	// the application is not found.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// GetUnitWorkloadStatus returns the workload status of the specified unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
	GetUnitWorkloadStatus(context.Context, coreunit.Name) (*corestatus.StatusInfo, error)

	// SetUnitWorkloadStatus sets the workload status of the specified unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
	SetUnitWorkloadStatus(context.Context, coreunit.Name, *corestatus.StatusInfo) error

	// GetApplicationStatus looks up the status of the specified application,
	// returning an error satisfying [applicationerrors.ApplicationNotFound] if the
	// application is not found.
	GetApplicationStatus(ctx context.Context, appID coreapplication.ID) (*corestatus.StatusInfo, error)

	// GetCharmModifiedVersion looks up the charm modified version of the given
	// application.
	GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error)

	// GetAvailableCharmArchiveSHA256 returns the SHA256 hash of the charm archive
	// for the given charm name, source and revision. If the charm is not available,
	// [applicationerrors.CharmNotResolved] is returned.
	GetAvailableCharmArchiveSHA256(ctx context.Context, locator charm.CharmLocator) (string, error)

	// GetCharmLXDProfile returns the LXD profile along with the revision of the
	// charm using the charm name, source and revision.
	GetCharmLXDProfile(ctx context.Context, locator charm.CharmLocator) (internalcharm.LXDProfile, charm.Revision, error)
}

// UnitStateService describes the ability to retrieve and persist
// unit agent state for informing hook reconciliation.
type UnitStateService interface {
	// GetUnitUUIDForName returns the UUID corresponding to the input unit name.
	GetUnitUUIDForName(ctx context.Context, name string) (string, error)
	// SetState persists the input unit state.
	SetState(context.Context, unitstate.UnitState) error
	// GetState returns the full unit state. The state may be empty.
	GetState(ctx context.Context, uuid string) (unitstate.RetrievedUnitState, error)
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
