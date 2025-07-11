// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/domain/relation"
)

// UpgradeService provides a subset of the upgrade domain service methods.
type UpgradeService interface {
	// IsUpgrading returns whether the controller is currently upgrading.
	IsUpgrading(context.Context) (bool, error)
}

// ControllerConfigService provides access to the controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)
}

// ControllerNodeService provides access to all known controller API addresses.
type ControllerNodeService interface {
	// GetAllAPIAddressesForClients returns a string slice of api
	// addresses available for agents.
	GetAllAPIAddressesForClients(ctx context.Context) ([]string, error)
}

// ModelInfoService provides access to information about the model.
type ModelInfoService interface {
	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(context.Context) (model.ModelInfo, error)
}

// ModelService provides access to currently deployed models.
type ModelService interface {
	// ControllerModel returns the model used for housing the Juju controller.
	ControllerModel(ctx context.Context) (model.Model, error)
	// ListAllModels returns all models registered in the controller. If no
	// models exist a zero value slice will be returned.
	ListAllModels(context.Context) ([]model.Model, error)
	// Model returns the model associated with the provided uuid.
	Model(ctx context.Context, uuid model.UUID) (model.Model, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// CheckAllApplicationsAndUnitsAreAlive checks that all applications and units
	// in the model are alive, returning an error if any are not.
	CheckAllApplicationsAndUnitsAreAlive(ctx context.Context) error

	// GetUnitNamesForApplication returns a slice of the unit names for the given application
	GetUnitNamesForApplication(ctx context.Context, appName string) ([]unit.Name, error)
}

// RelationService provides access to the relation service.
type RelationService interface {
	// GetAllRelationDetails return RelationDetailResults for all relations
	// for the current model.
	GetAllRelationDetails(ctx context.Context) ([]relation.RelationDetailsResult, error)

	// RelationUnitInScopeByID returns a boolean to indicate whether the given
	// unit is in scopen of a given relation
	RelationUnitInScopeByID(ctx context.Context, relationID int, unitName unit.Name) (bool,
		error)
}

type StatusService interface {
	// CheckUnitStatusesReadyForMigration returns true is the statuses of all units
	// in the model indicate they can be migrated.
	CheckUnitStatusesReadyForMigration(context.Context) error

	// CheckMachineStatusesReadyForMigration returns an error if the statuses of any
	// machines in the model indicate they cannot be migrated.
	CheckMachineStatusesReadyForMigration(context.Context) error
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetMachinesNotAtTargetAgentVersion reports all of the machines in the model that
	// are currently not at the desired target version. This also returns machines
	// that have no reported agent version set. If all units are up to the
	// target version or no units exist in the model a zero length slice is
	// returned.
	GetMachinesNotAtTargetAgentVersion(context.Context) ([]machine.Name, error)

	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/modelagent/errors.NotFound] when the model
	// does not exist.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// GetUnitsNotAtTargetAgentVersion reports all of the units in the model that
	// are currently not at the desired target agent version. This also returns
	// units that have no reported agent version set. If all units are up to the
	// target version or no units exist in the model a zero length slice is
	// returned.
	GetUnitsNotAtTargetAgentVersion(context.Context) ([]unit.Name, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}

// ModelMigrationService provides access to migration status.
type ModelMigrationService interface {
	// ModelMigrationMode returns the current migration mode for the model.
	ModelMigrationMode(ctx context.Context) (modelmigration.MigrationMode, error)
	// WatchForMigration returns a notification watcher that fires when this
	// model undergoes migration.
	WatchForMigration(ctx context.Context) (watcher.NotifyWatcher, error)
	// Migration returns status about migration of this model.
	Migration(ctx context.Context) (modelmigration.Migration, error)
	// SetMigrationPhase is called by the migration master to progress migration.
	SetMigrationPhase(ctx context.Context, phase migration.Phase) error
	// SetMigrationStatusMessage is called by the migration master to report on
	// migration status.
	SetMigrationStatusMessage(ctx context.Context, message string) error
	// WatchMinionReports returns a notification watcher that fires when any minion
	// reports a update to their phase.
	WatchMinionReports(ctx context.Context) (watcher.NotifyWatcher, error)
	// MinionReports returns phase information about minions in this model.
	MinionReports(ctx context.Context) (migration.MinionReports, error)
}

// MachineService is used to get the life of all machines before migrating.
type MachineService interface {
	// AllMachineNames returns the names of all machines in the model.
	AllMachineNames(ctx context.Context) ([]machine.Name, error)
	// GetMachineLife returns the GetMachineLife status of the specified machine.
	// It returns a NotFound if the given machine doesn't exist.
	GetMachineLife(ctx context.Context, machineName machine.Name) (life.Value, error)
	// GetMachineBase returns the base for the given machine.
	//
	// The following errors may be returned:
	// - [machineerrors.MachineNotFound] if the machine does not exist.
	GetMachineBase(ctx context.Context, mName machine.Name) (base.Base, error)
}
