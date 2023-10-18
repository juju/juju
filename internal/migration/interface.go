// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/names/v4"
	"github.com/juju/replicaset/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/credential"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/state"
)

// PrecheckBackend defines the interface to query Juju's state
// for migration prechecks.
type PrecheckBackend interface {
	AgentVersion() (version.Number, error)
	NeedsCleanup() (bool, error)
	Model() (PrecheckModel, error)
	AllModelUUIDs() ([]string, error)
	IsUpgrading() (bool, error)
	IsMigrationActive(string) (bool, error)
	AllMachines() ([]PrecheckMachine, error)
	AllApplications() ([]PrecheckApplication, error)
	AllRelations() ([]PrecheckRelation, error)
	AllCharmURLs() ([]*string, error)
	ControllerBackend() (PrecheckBackend, error)
	HasUpgradeSeriesLocks() (bool, error)
	MachineCountForBase(base ...state.Base) (map[string]int, error)
	MongoCurrentStatus() (*replicaset.Status, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	CloudCredential(ctx context.Context, id credential.ID) (cloud.Credential, error)
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
	AgentVersion() (version.Number, error)
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
	ShouldRebootOrShutdown() (state.RebootAction, error)
}

// PrecheckApplication describes the state interface for an
// application needed by migration prechecks.
type PrecheckApplication interface {
	Name() string
	Life() state.Life
	CharmURL() (*string, bool)
	AllUnits() ([]PrecheckUnit, error)
	MinUnits() int
}

// PrecheckUnit describes state interface for a unit needed by
// migration prechecks.
type PrecheckUnit interface {
	Name() string
	AgentTools() (*tools.Tools, error)
	Life() state.Life
	CharmURL() *string
	AgentStatus() (status.StatusInfo, error)
	Status() (status.StatusInfo, error)
	ShouldBeAssigned() bool
	IsSidecar() (bool, error)
}

// PrecheckRelation describes the state interface for relations needed
// for prechecks.
type PrecheckRelation interface {
	String() string
	Endpoints() []state.Endpoint
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

// ModelPresence represents the API server connections for a model.
type ModelPresence interface {
	// For a given non controller agent, return the Status for that agent.
	AgentStatus(agent string) (presence.Status, error)
}

type environsCloudSpecGetter func(names.ModelTag) (environscloudspec.CloudSpec, error)
