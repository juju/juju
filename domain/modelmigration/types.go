// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"time"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
)

// MigrationMachineDiscrepancy describes a divergent machine between what Juju
// has and what the cloud has reported. If both the MachineName and the
// CloudInstanceId are both not empty then the discrepancy is on the Juju side
// where we are referencing a instance that doesn't exist in the cloud.
//
// If MachineName is empty then the discrepancy comes from the cloud where a
// instance exists that is not being tracked by Juju.
type MigrationMachineDiscrepancy struct {
	// MachineName is the name given to a machine in the Juju model
	MachineName machine.Name

	// CloudInstanceId is the unique id given to an instance from the cloud.
	CloudInstanceId instance.Id
}

// MigrationMode specifies where the Model is with respect to migration.
type MigrationMode string

const (
	// MigrationModeNone is the default mode for a model and reflects
	// that it isn't involved with a model migration.
	MigrationModeNone = MigrationMode("")

	// MigrationModeExporting reflects a model that is in the process of being
	// exported from one controller to another.
	MigrationModeExporting = MigrationMode("exporting")

	// MigrationModeImporting reflects a model that is being imported into a
	// controller, but is not yet fully active.
	MigrationModeImporting = MigrationMode("importing")
)

type Migration struct {
	UUID             string
	Phase            migration.Phase
	PhaseChangedTime time.Time
	Target           migration.TargetInfo
}

// MigrationSpec describes the details required to initiate and record a new
// model migration export on the source controller. It is the input to
// [github.com/juju/juju/domain/modelmigration/state/controller.State.InsertExport].
type MigrationSpec struct {
	// MigrationUUID uniquely identifies this migration attempt. A retry is a
	// brand new migration with a new UUID, not a resumption of a previous one.
	MigrationUUID string

	// ModelUUID is the UUID of the model being migrated.
	ModelUUID string

	// Target carries the connection and authentication details for the
	// controller the model is being migrated to.
	Target migration.TargetInfo
}

// ExternalControllerInfo describes the connection details for an external
// controller that must exist (or be created) before a migration row that
// references it can be written. It is the input to the shared
// compare-or-insert helper
// [github.com/juju/juju/domain/modelmigration/state/controller.State.EnsureExternalControllerMatchesOrInsert].
type ExternalControllerInfo struct {
	// UUID is the external controller's unique identifier.
	UUID string

	// Alias is an optional human-friendly name for the controller.
	Alias string

	// CACert is the CA certificate used to validate the controller's API
	// server certificate.
	CACert string

	// Addresses holds the API server addresses for the controller.
	Addresses []string
}

// MinionReports is the aggregated set of minion phase reports recorded against
// a migration for a single phase. It carries the reported entities only; the
// set of agents that have not yet reported (the "unknown" set) is derived by
// the service from the model's agent inventory.
type MinionReports struct {
	// Phase is the migration phase the reports relate to.
	Phase migration.Phase

	// Succeeded holds the entity keys (agent tag strings) of agents that have
	// reported success for the phase.
	Succeeded []string

	// Failed holds the entity keys (agent tag strings) of agents that have
	// reported failure for the phase.
	Failed []string
}
