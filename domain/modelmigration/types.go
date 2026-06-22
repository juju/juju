// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"time"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/internal/errors"
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

// ImportPhase is the phase of a target-side import claim, mirroring the
// model_migration_import_phase_type lookup table.
type ImportPhase string

const (
	// ImportPhaseImporting reflects an import claim whose model content is
	// still being written; Abort is allowed.
	ImportPhaseImporting = ImportPhase("importing")

	// ImportPhaseActivating reflects an import claim that has crossed the
	// activation point of no return; Abort is forbidden.
	ImportPhaseActivating = ImportPhase("activating")

	// ImportPhaseAborting reflects an import claim whose partial state is
	// being cleaned up; Import and Activate are forbidden.
	ImportPhaseAborting = ImportPhase("aborting")
)

// ImportClaim describes an existing target-side import claim
// (model_migration_import row) for a model UUID.
type ImportClaim struct {
	// SourceMigrationUUID is the migration UUID recorded by the source side
	// when the claim was created. It is diagnostic only.
	SourceMigrationUUID string

	// Phase is the claim's current import phase.
	Phase ImportPhase

	// UpdatedAt is when the claim last changed phase.
	UpdatedAt time.Time
}

// ImportClaimConflictError builds the coded AlreadyExists error returned when
// a v8 import claim attempt finds an existing claim for modelUUID. The
// message reflects the existing claim's phase: activation-in-progress
// wording when phase=activating (the source must retry/continue Activate
// rather than Abort), cleanup-in-progress wording when phase=aborting, or a
// plain occupied-model message for a duplicate importing claim. Both
// [service.Service.BeginImport] and the v8 import driver
// (internal/migration.ModelImporter.ImportModelV2) use this so the wording
// stays identical regardless of which caller observes the conflict.
func ImportClaimConflictError(modelUUID string, phase ImportPhase) error {
	switch phase {
	case ImportPhaseActivating:
		return errors.Errorf(
			"model import for %s: activation in progress: %w", modelUUID, coreerrors.AlreadyExists)
	case ImportPhaseAborting:
		return errors.Errorf(
			"model import for %s: cleanup in progress: %w", modelUUID, coreerrors.AlreadyExists)
	default:
		return errors.Errorf("model import for %s: %w", modelUUID, coreerrors.AlreadyExists)
	}
}

// ImportPrecheckArgs carries the target-portable semantic data from a v8
// migration envelope that the target controller validates before accepting an
// import. It is assembled by the migrationtarget facade from the typed
// envelope fields and consumed by the modelmigration import prechecks, which
// query the target controller database directly.
type ImportPrecheckArgs struct {
	// ModelUUID, ModelName and ModelQualifier identify the migrating model and
	// drive the model collision checks.
	ModelUUID      string
	ModelName      string
	ModelQualifier string

	// Cloud is the name of the cloud the model runs on; it must exist on the
	// target controller.
	Cloud string

	// CloudRegion is the model's cloud region. When non-empty it must be a
	// region known to Cloud on the target controller.
	CloudRegion string

	// Users are the names of the controller users referenced by the model. A
	// user missing from the target is fine (it is recreated on import); an
	// existing user must not be disabled.
	Users []string

	// Credential is the model's cloud credential, or nil when the model has
	// none. When the credential already exists on the target it must not be
	// revoked.
	Credential *ImportPrecheckCredential

	// SecretBackend is the name of the model's secret backend, or empty when
	// the model has none. When set it must exist on the target controller.
	SecretBackend string
}

// ImportModelCollision reports target-side model identity collisions that
// block importing a model.
type ImportModelCollision struct {
	// Importing is true when a target-side import row already exists for the
	// model UUID.
	Importing bool

	// ModelExists is true when a model row already exists for the model UUID.
	ModelExists bool

	// ModelNamespaceExists is true when a model database namespace already
	// exists for the model UUID.
	ModelNamespaceExists bool

	// ModelNameExists is true when the model name and qualifier are already in
	// use.
	ModelNameExists bool
}

// ImportPrecheckCredential is the natural key plus revoked status of the
// model's cloud credential, used by the import prechecks to compare against
// any credential already present on the target controller.
type ImportPrecheckCredential struct {
	Cloud   string
	Owner   string
	Name    string
	Revoked bool
}
