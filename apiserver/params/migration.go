// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// InitiateModelMigrationArgs holds the details required to start one
// or more model migrations.
type InitiateModelMigrationArgs struct {
	Specs []ModelMigrationSpec `json:"specs"`
}

// ModelMigrationSpec holds the details required to start the
// migration of a single model.
type ModelMigrationSpec struct {
	ModelTag   string                   `json:"model-tag"`
	TargetInfo ModelMigrationTargetInfo `json:"target-info"`
}

// ModelMigrationTargetInfo holds the details required to connect to
// and authenticate with a remote controller for model migration.
type ModelMigrationTargetInfo struct {
	ControllerTag string   `json:"controller-tag"`
	Addrs         []string `json:"addrs"`
	CACert        string   `json:"ca-cert"`
	AuthTag       string   `json:"auth-tag"`
	Password      string   `json:"password"`
}

// InitiateModelMigrationResults is used to return the result of one
// or more attempts to start model migrations.
type InitiateModelMigrationResults struct {
	Results []InitiateModelMigrationResult `json:"results"`
}

// InitiateModelMigrationResult is used to return the result of one
// model migration initiation attempt.
type InitiateModelMigrationResult struct {
	ModelTag string `json:"model-tag"`
	Error    *Error `json:"error"`
	Id       string `json:"id"` // the ID for the migration attempt
}

// SetMigrationPhaseArgs provides a migration phase to the
// migrationmaster.SetPhase API method.
type SetMigrationPhaseArgs struct {
	Phase string `json:"phase"`
}

// SerializedModel wraps a buffer contain a serialised Juju model. It
// also contains lists of the charms and tools used in the model.
type SerializedModel struct {
	Bytes  []byte   `json:"bytes"`
	Charms []string `json:"charms"`
}

// ModelArgs wraps a simple model tag.
type ModelArgs struct {
	ModelTag string `json:"model-tag"`
}

// MigrationStatus reports the current status of a model migration.
type MigrationStatus struct {
	Attempt int    `json:"attempt"`
	Phase   string `json:"phase"`

	// TODO(mjs): I'm not convinced these Source fields will get used.
	SourceAPIAddrs []string `json:"source-api-addrs"`
	SourceCACert   string   `json:"source-ca-cert"`

	TargetAPIAddrs []string `json:"target-api-addrs"`
	TargetCACert   string   `json:"target-ca-cert"`
}

// FullMigrationStatus reports the current status of a model
// migration, including authentication details for the remote
// controller.
type FullMigrationStatus struct {
	Spec    ModelMigrationSpec `json:"spec"`
	Attempt int                `json:"attempt"`
	Phase   string             `json:"phase"`
}

type PhaseResult struct {
	Phase string `json:"phase"`
	Error *Error
}

type PhaseResults struct {
	Results []PhaseResult `json:"Results"`
}
