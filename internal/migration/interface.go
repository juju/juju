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
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/relation"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
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
	AllApplications() ([]PrecheckApplication, error)
	AllRelations() ([]PrecheckRelation, error)
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
	// GetApplicationLife looks up the life of the specified application,
	// returning an error satisfying
	// [applicationerrors.ApplicationNotFoundError] if the application is not
	// found.
	GetApplicationLife(context.Context, string) (life.Value, error)
}

type StatusService interface {
	// CheckUnitStatusesReadyForMigration returns true is the statuses of all units
	// in the model indicate they can be migrated.
	CheckUnitStatusesReadyForMigration(context.Context) error
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
	MigrationMode() state.MigrationMode
	CloudCredentialTag() (names.CloudCredentialTag, bool)
}

// PrecheckMachine describes the state interface for a machine needed
// by migration prechecks.
type PrecheckMachine interface {
	Id() string
	AgentTools() (*tools.Tools, error)
	Life() state.Life
	Status() (status.StatusInfo, error)
	InstanceStatus() (status.StatusInfo, error)
	// TODO(gfouillet): Restore this once machine fully migrated to dqlite
	// ShouldRebootOrShutdown() (state.RebootAction, error)
}

// PrecheckApplication describes the state interface for an
// application needed by migration prechecks.
type PrecheckApplication interface {
	Name() string
	CharmURL() (*string, bool)
	AllUnits() ([]PrecheckUnit, error)
}

// PrecheckUnit describes state interface for a unit needed by
// migration prechecks.
type PrecheckUnit interface {
	Name() string
	AgentTools() (*tools.Tools, error)
	Life() state.Life
	CharmURL() *string
	ShouldBeAssigned() bool
}

// PrecheckRelation describes the state interface for relations needed
// for prechecks.
type PrecheckRelation interface {
	String() string
	Endpoints() []relation.Endpoint
	Unit(PrecheckUnit) (PrecheckRelationUnit, error)
	AllRemoteUnits(appName string) ([]PrecheckRelationUnit, error)
	RemoteApplication() (string, bool, error)
}

// PrecheckRelationUnit describes the interface for relation units
// needed for migration prechecks.
type PrecheckRelationUnit interface {
	Valid() (bool, error)
	InScope() (bool, error)
	UnitName() string
}

type environsCloudSpecGetter func(context.Context, names.ModelTag) (environscloudspec.CloudSpec, error)
