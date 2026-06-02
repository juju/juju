// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import "time"

// entityUUID represents a generic uuid column from a given table in the
// model's database.
type entityUUID struct {
	UUID string `db:"uuid"`
}

// controllerTargetVersion represents the current target version set for the
// controller.
type controllerTargetVersion struct {
	TargetVersion string `db:"target_version"`
}

// modelUUIDArg is a query argument holding a model uuid.
type modelUUIDArg struct {
	ModelUUID string `db:"model_uuid"`
}

// migrationUUIDArg is a query argument holding a migration uuid (the export
// migration's primary key, referenced as migration_uuid by child tables).
type migrationUUIDArg struct {
	MigrationUUID string `db:"migration_uuid"`
}

// migrationExport maps the model_migration_export row for an export attempt.
type migrationExport struct {
	UUID                 string    `db:"uuid"`
	ModelUUID            string    `db:"model_uuid"`
	TargetControllerUUID string    `db:"target_controller_uuid"`
	CurrentPhaseID       int       `db:"current_phase_id"`
	PhaseChangedAt       time.Time `db:"phase_changed_at"`
	StartTime            time.Time `db:"start_time"`
}

// migrationTargetAuth maps a model_migration_export_target_auth row holding the
// per-migration credentials used to connect to the target controller.
type migrationTargetAuth struct {
	MigrationUUID          string `db:"migration_uuid"`
	ExternalControllerUUID string `db:"external_controller_uuid"`
	TargetUser             string `db:"target_user"`
	TargetMacaroons        string `db:"target_macaroons"`
	TargetToken            string `db:"target_token"`
	TargetSkipUserChecks   bool   `db:"target_skip_user_checks"`
}

// migrationPhaseEntry maps a model_migration_export_phase history row.
type migrationPhaseEntry struct {
	MigrationUUID string    `db:"migration_uuid"`
	PhaseID       int       `db:"phase_id"`
	ChangedAt     time.Time `db:"changed_at"`
}

// migrationStatus maps a model_migration_export_status row.
type migrationStatus struct {
	UUID          string    `db:"uuid"`
	MigrationUUID string    `db:"migration_uuid"`
	Message       string    `db:"message"`
	RecordedAt    time.Time `db:"recorded_at"`
}

// migrationMinionSync maps a model_migration_export_minion_sync row.
type migrationMinionSync struct {
	MigrationUUID string    `db:"migration_uuid"`
	PhaseID       int       `db:"phase_id"`
	EntityKey     string    `db:"entity_key"`
	Success       bool      `db:"success"`
	ReportedAt    time.Time `db:"reported_at"`
}

// minionReportRow is the projection read back when aggregating minion reports.
type minionReportRow struct {
	EntityKey string `db:"entity_key"`
	Success   bool   `db:"success"`
}

// phaseIDArg holds a single migration phase lookup id.
type phaseIDArg struct {
	PhaseID int `db:"phase_id"`
}

// currentPhase is the projection of the export's denormalised current phase.
type currentPhase struct {
	CurrentPhaseID int `db:"current_phase_id"`
}

// phaseUpdate carries the arguments for an optimistic phase update.
type phaseUpdate struct {
	UUID            string    `db:"uuid"`
	NewPhaseID      int       `db:"new_phase_id"`
	ExpectedPhaseID int       `db:"expected_phase_id"`
	PhaseChangedAt  time.Time `db:"phase_changed_at"`
}

// endExport carries the arguments for marking an export as ended.
type endExport struct {
	UUID           string    `db:"uuid"`
	PhaseID        int       `db:"current_phase_id"`
	PhaseChangedAt time.Time `db:"phase_changed_at"`
	EndTime        time.Time `db:"end_time"`
}

// externalControllerUpsert maps a model_migration external_controller row used
// by the compare-or-insert helper.
type externalControllerUpsert struct {
	UUID   string `db:"uuid"`
	Alias  string `db:"alias"`
	CACert string `db:"ca_cert"`
}

// externalControllerCACert is the projection used to compare an existing
// external controller's CA certificate.
type externalControllerCACert struct {
	CACert string `db:"ca_cert"`
}

// externalControllerAddress maps a model_migration external_controller_address
// row.
type externalControllerAddress struct {
	UUID           string `db:"uuid"`
	ControllerUUID string `db:"controller_uuid"`
	Address        string `db:"address"`
}

// addressValue is the projection of a single external controller address.
type addressValue struct {
	Address string `db:"address"`
}

// countResult holds a COUNT(*) projection.
type countResult struct {
	Count int `db:"count"`
}
