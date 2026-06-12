// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"time"
)

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
	UpdatedAt            time.Time `db:"updated_at"`
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
	ModelUUID     string    `db:"model_uuid"`
	PhaseID       int       `db:"phase_id"`
	ChangedAt     time.Time `db:"changed_at"`
}

// migrationStatus maps a model_migration_export_status row.
type migrationStatus struct {
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
	CurrentPhaseID int    `db:"current_phase_id"`
	ModelUUID      string `db:"model_uuid"`
}

// terminalPhaseIDArgs carries the persisted ids for terminal export phases.
type terminalPhaseIDArgs struct {
	ReapFailedID int `db:"reap_failed_id"`
	DoneID       int `db:"done_id"`
	AbortDoneID  int `db:"abort_done_id"`
}

// phaseUpdate carries the arguments for an optimistic phase update.
type phaseUpdate struct {
	UUID            string    `db:"uuid"`
	NewPhaseID      int       `db:"new_phase_id"`
	ExpectedPhaseID int       `db:"expected_phase_id"`
	UpdatedAt       time.Time `db:"updated_at"`
}

// endExport carries the arguments for marking an export as ended.
type endExport struct {
	UUID      string    `db:"uuid"`
	PhaseID   int       `db:"current_phase_id"`
	UpdatedAt time.Time `db:"updated_at"`
}

// externalControllerInfo maps a model_migration external_controller row.
type externalControllerInfo struct {
	UUID   string `db:"uuid"`
	Alias  string `db:"alias"`
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

// modelIdentityRow is the model's bootstrap identity with cloud, region,
// credential and life resolved to natural keys. Region and credential columns
// are nullable.
type modelIdentityRow struct {
	UUID            string  `db:"uuid"`
	Name            string  `db:"name"`
	Qualifier       string  `db:"qualifier"`
	Type            string  `db:"model_type"`
	Cloud           string  `db:"cloud"`
	CloudRegion     *string `db:"cloud_region"`
	CredentialName  *string `db:"credential_name"`
	CredentialOwner *string `db:"credential_owner"`
	Life            string  `db:"life"`
}

// permissionRow is a single model or offer permission grant with the grantee
// resolved to a username and the object/access types resolved to their string
// values.
type permissionRow struct {
	ObjectType  string `db:"object_type"`
	GrantOn     string `db:"grant_on"`
	SubjectName string `db:"subject_name"`
	Access      string `db:"access"`
}

// userRow is the non-authentication profile of a user, with the user's
// last login against the model joined in (null when never logged in).
type userRow struct {
	Name        string     `db:"name"`
	DisplayName *string    `db:"display_name"`
	CreatedBy   *string    `db:"created_by"`
	CreatedAt   time.Time  `db:"created_at"`
	Removed     bool       `db:"removed"`
	External    bool       `db:"external"`
	LastLogin   *time.Time `db:"last_login"`
}

// credentialRow is a model cloud credential's natural key, auth type and
// status, joined with its auth attributes (one row per attribute; the
// attribute columns are null for a credential without attributes).
type credentialRow struct {
	Cloud         string  `db:"cloud"`
	Owner         string  `db:"owner"`
	Name          string  `db:"name"`
	AuthType      string  `db:"auth_type"`
	Revoked       *bool   `db:"revoked"`
	Invalid       *bool   `db:"invalid"`
	InvalidReason *string `db:"invalid_reason"`
	AttrKey       *string `db:"attr_key"`
	AttrValue     *string `db:"attr_value"`
}

// authorizedKeyRow is a single SSH public key authorised for the model, with
// its owner resolved to a username.
type authorizedKeyRow struct {
	Username  string `db:"username"`
	PublicKey string `db:"public_key"`
}

// modelSecretBackendRow is the model's secret backend resolved to its name and
// type.
type modelSecretBackendRow struct {
	Name        string `db:"name"`
	BackendType string `db:"backend_type"`
}

// secretBackendRefRow maps a model secret revision to its backend, by name.
type secretBackendRefRow struct {
	BackendName        string `db:"backend_name"`
	SecretRevisionUUID string `db:"secret_revision_uuid"`
	SecretID           string `db:"secret_id"`
}

// leadershipRow is an application-leadership lease holder. Name and holder are
// nullable in the schema.
type leadershipRow struct {
	Name   *string `db:"name"`
	Holder *string `db:"holder"`
}

// leaseTypeArg selects leases by their type name.
type leaseTypeArg struct {
	Type string `db:"type"`
}

// cloudImageMetadataSource is the source selector for cloud image metadata.
type cloudImageMetadataSource struct {
	Source string `db:"source"`
}

// cloudImageMetadataRow is a custom cloud image metadata row with the
// architecture resolved to its name.
type cloudImageMetadataRow struct {
	Stream          string    `db:"stream"`
	Region          string    `db:"region"`
	Version         string    `db:"version"`
	Arch            string    `db:"arch"`
	VirtType        string    `db:"virt_type"`
	RootStorageType string    `db:"root_storage_type"`
	RootStorageSize *uint64   `db:"root_storage_size"`
	Source          string    `db:"source"`
	Priority        int       `db:"priority"`
	ImageID         string    `db:"image_id"`
	CreatedAt       time.Time `db:"created_at"`
}

// externalControllerRow is a third-party external controller's connection
// identity.
type externalControllerRow struct {
	UUID   string  `db:"uuid"`
	Alias  *string `db:"alias"`
	CACert string  `db:"ca_cert"`
}

// externalControllerAddressRow is a single address for an external controller.
type externalControllerAddressRow struct {
	ControllerUUID string `db:"controller_uuid"`
	Address        string `db:"address"`
}

// externalModelRow is a third-party model hosted by an external controller.
type externalModelRow struct {
	ControllerUUID string `db:"controller_uuid"`
	ModelUUID      string `db:"model_uuid"`
}

type externalModelKey struct {
	controllerUUID string
	modelUUID      string
}
