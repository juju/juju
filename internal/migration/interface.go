// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/state"
)

type ModelService interface {
	// ListAllModels returns all models registered in the controller. If no
	// models exist a zero value slice will be returned.
	ListAllModels(context.Context) ([]coremodel.Model, error)
	// Model returns the model associated with the provided uuid.
	Model(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error)
}

// ModelMigrationService provides access to migration status.
type ModelMigrationService interface {
	// ModelMigrationMode returns the current migration mode for the model.
	ModelMigrationMode(ctx context.Context) (modelmigration.MigrationMode, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}

// UpgradeService provides access to upgrade information.
type UpgradeService interface {
	IsUpgrading(context.Context) (bool, error)
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

// ControllerConfigService describes the method needed to get the
// controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// PrecheckMachine describes the state interface for a machine needed
// by migration prechecks.
type PrecheckMachine interface {
	Id() string
	AgentTools() (*tools.Tools, error)
	Life() state.Life
	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	// ShouldRebootOrShutdown() (state.RebootAction, error)
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
