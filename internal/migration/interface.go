// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/replicaset/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/state"
)

// PrecheckBackend defines the interface to query Juju's state
// for migration prechecks.
type PrecheckBackend interface {
	NeedsCleanup() (bool, error)
	Model() (PrecheckModel, error)
	AllModelUUIDs() ([]string, error)
	IsMigrationActive(string) (bool, error)
	AllMachines() ([]PrecheckMachine, error)
	AllMachinesCount() (int, error)
	ControllerBackend() (PrecheckBackend, error)
	MachineCountForBase(base ...state.Base) (map[string]int, error)
	MongoCurrentStatus() (*replicaset.Status, error)
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

// Pool defines the interface to a StatePool used by the migration
// prechecks.
type Pool interface {
	GetModel(string) (PrecheckModel, func(), error)
}

// PrecheckModel describes the state interface a model as needed by
// the migration prechecks.
type PrecheckModel interface {
	UUID() string
	Name() string
	Type() state.ModelType
	Owner() names.UserTag
	Life() state.Life
	MigrationMode() (state.MigrationMode, error)
	CloudCredentialTag() (names.CloudCredentialTag, bool)
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
