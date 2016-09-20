// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/version"
)

// InitiateMigrationArgs holds the details required to start one or
// more model migrations.
type InitiateMigrationArgs struct {
	Specs []MigrationSpec `json:"specs"`
}

// MigrationSpec holds the details required to start the migration of
// a single model.
type MigrationSpec struct {
	ModelTag        string              `json:"model-tag"`
	TargetInfo      MigrationTargetInfo `json:"target-info"`
	ExternalControl bool                `json:"external-control"`

	// SkipInitialPrechecks allows the migration prechecks run during
	// handling of the InitiateMigration API call to be bypassed. It
	// is only honoured if ExternalControl is true.
	SkipInitialPrechecks bool `json:"skip-initial-prechecks"`
}

// MigrationTargetInfo holds the details required to connect to and
// authenticate with a remote controller for model migration.
type MigrationTargetInfo struct {
	ControllerTag string   `json:"controller-tag"`
	Addrs         []string `json:"addrs"`
	CACert        string   `json:"ca-cert"`
	AuthTag       string   `json:"auth-tag"`
	Password      string   `json:"password,omitempty"`
	Macaroons     string   `json:"macaroons,omitempty"`
}

// InitiateMigrationResults is used to return the result of one or
// more attempts to start model migrations.
type InitiateMigrationResults struct {
	Results []InitiateMigrationResult `json:"results"`
}

// InitiateMigrationResult is used to return the result of one model
// migration initiation attempt.
type InitiateMigrationResult struct {
	ModelTag    string `json:"model-tag"`
	Error       *Error `json:"error,omitempty"`
	MigrationId string `json:"migration-id"`
}

// SetMigrationPhaseArgs provides a migration phase to the
// migrationmaster.SetPhase API method.
type SetMigrationPhaseArgs struct {
	Phase string `json:"phase"`
}

// SetMigrationStatusMessageArgs provides a migration status message
// to the migrationmaster.SetStatusMessage API method.
type SetMigrationStatusMessageArgs struct {
	Message string `json:"message"`
}

// SerializedModel wraps a buffer contain a serialised Juju model. It
// also contains lists of the charms and tools used in the model.
type SerializedModel struct {
	Bytes  []byte                 `json:"bytes"`
	Charms []string               `json:"charms"`
	Tools  []SerializedModelTools `json:"tools"`
}

// SerializedModelTools holds the version and URI for a given tools
// version.
type SerializedModelTools struct {
	Version string `json:"version"`

	// URI holds the URI were a client can download the tools
	// (e.g. "/tools/1.2.3-xenial-amd64"). It will need to prefixed
	// with the API server scheme, address and model prefix before it
	// can be used.
	URI string `json:"uri"`
}

// ModelArgs wraps a simple model tag.
type ModelArgs struct {
	ModelTag string `json:"model-tag"`
}

// MasterMigrationStatus is used to report the current status of a
// model migration for the migrationmaster. It includes authentication
// details for the remote controller.
type MasterMigrationStatus struct {
	Spec             MigrationSpec `json:"spec"`
	MigrationId      string        `json:"migration-id"`
	Phase            string        `json:"phase"`
	PhaseChangedTime time.Time     `json:"phase-changed-time"`
}

// MigrationModelInfo is used to report basic model information to the
// migrationmaster worker.
type MigrationModelInfo struct {
	UUID         string         `json:"uuid"`
	Name         string         `json:"name"`
	OwnerTag     string         `json:"owner-tag"`
	AgentVersion version.Number `json:"agent-version"`
}

// MigrationStatus reports the current status of a model migration.
type MigrationStatus struct {
	MigrationId     string `json:"migration-id"`
	Attempt         int    `json:"attempt"`
	Phase           string `json:"phase"`
	ExternalControl bool   `json:"external-control"`

	// TODO(mjs): I'm not convinced these Source fields will get used.
	SourceAPIAddrs []string `json:"source-api-addrs"`
	SourceCACert   string   `json:"source-ca-cert"`

	TargetAPIAddrs []string `json:"target-api-addrs"`
	TargetCACert   string   `json:"target-ca-cert"`
}

// PhasesResults holds the phase of one or more model migrations.
type PhaseResults struct {
	Results []PhaseResult `json:"results"`
}

// PhaseResult holds the phase of a single model migration, or an
// error if the phase could not be determined.
type PhaseResult struct {
	Phase string `json:"phase,omitempty"`
	Error *Error `json:"error,omitempty"`
}

// MinionReport holds the details of whether a migration minion
// succeeded or failed for a specific migration phase.
type MinionReport struct {
	// MigrationId holds the id of the migration the agent is
	// reporting about.
	MigrationId string `json:"migration-id"`

	// Phase holds the phase of the migration the agent is
	// reporting about.
	Phase string `json:"phase"`

	// Success is true if the agent successfully completed its actions
	// for the migration phase, false otherwise.
	Success bool `json:"success"`
}

// MinionReports holds the details of whether a migration minion
// succeeded or failed for a specific migration phase.
type MinionReports struct {
	// MigrationId holds the id of the migration the reports related to.
	MigrationId string `json:"migration-id"`

	// Phase holds the phase of the migration the reports related to.
	Phase string `json:"phase"`

	// SuccessCount holds the number of agents which have successfully
	// completed a given migration phase.
	SuccessCount int `json:"success-count"`

	// UnknownCount holds the number of agents still to report for a
	// given migration phase.
	UnknownCount int `json:"unknown-count"`

	// UnknownSample holds the tags of a limited number of agents
	// that are still to report for a given migration phase (for
	// logging or showing in a user interface).
	UnknownSample []string `json:"unknown-sample"`

	// Failed contains the tags of all agents which have reported a
	// failed to complete a given migration phase.
	Failed []string `json:"failed"`
}
