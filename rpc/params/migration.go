// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/core/semversion"
)

// MigrationModelHTTPHeader is the key for the HTTP header value
// that is used to specify the model UUID for the model being migrated
// for the uploading of the binaries for that model.
const MigrationModelHTTPHeader = "X-Juju-Migration-Model-UUID"

// InitiateMigrationArgs holds the details required to start one or
// more model migrations.
type InitiateMigrationArgs struct {
	Specs []MigrationSpec `json:"specs"`
}

// MigrationSpec holds the details required to start the migration of
// a single model.
type MigrationSpec struct {
	ModelTag   string              `json:"model-tag"`
	TargetInfo MigrationTargetInfo `json:"target-info"`
}

// MigrationTargetInfo holds the details required to connect to and
// authenticate with a remote controller for model migration.
type MigrationTargetInfo struct {
	ControllerTag   string   `json:"controller-tag"`
	ControllerAlias string   `json:"controller-alias,omitempty"`
	Addrs           []string `json:"addrs"`
	CACert          string   `json:"ca-cert"`
	AuthTag         string   `json:"auth-tag"`
	Password        string   `json:"password,omitempty"`
	Macaroons       string   `json:"macaroons,omitempty"`
}

// MigrationSourceInfo holds the details required to connect to
// the source controller for model migration.
type MigrationSourceInfo struct {
	LocalRelatedModels []string `json:"local-related-models"`
	ControllerTag      string   `json:"controller-tag"`
	ControllerAlias    string   `json:"controller-alias,omitempty"`
	Addrs              []string `json:"addrs"`
	CACert             string   `json:"ca-cert"`
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

// PrechecksArgs provides the target controller version
// to the migrationmaster.Prechecks API method.
type PrechecksArgs struct {
	TargetControllerVersion semversion.Number `json:"target-controller-version"`
}

// SerializedModel wraps a buffer contain a serialised Juju model. It
// also contains lists of the charms and tools used in the model.
type SerializedModel struct {
	Bytes     []byte                    `json:"bytes"`
	Charms    []string                  `json:"charms"`
	Tools     []SerializedModelTools    `json:"tools"`
	Resources []SerializedModelResource `json:"resources"`
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

	// SHA256 is the sha 256 sum of the tools backed by the URI.
	SHA256 string
}

// SerializedModelResource holds the details for a single resource for
// an application in a serialized model.
type SerializedModelResource struct {
	Application    string    `json:"application"`
	Name           string    `json:"name"`
	Revision       int       `json:"revision"`
	Type           string    `json:"type"`
	Origin         string    `json:"origin"`
	FingerprintHex string    `json:"fingerprint"`
	Size           int64     `json:"size"`
	Timestamp      time.Time `json:"timestamp"`
	Username       string    `json:"username,omitempty"`
}

// ModelArgs wraps a simple model tag.
type ModelArgs struct {
	ModelTag string `json:"model-tag"`
}

// ActivateModelArgs holds args used to
// activate a newly migrated model.
type ActivateModelArgs struct {
	// ModelTag is the model being migrated.
	ModelTag string `json:"model-tag"`

	// ControllerTag is the tag of the source controller.
	ControllerTag string `json:"controller-tag"`
	// ControllerAlias is the name of the source controller.
	ControllerAlias string `json:"controller-alias,omitempty"`
	// SourceAPIAddrs are the api addresses of the source controller.
	SourceAPIAddrs []string `json:"source-api-addrs"`
	// SourceCACert is the CA certificate used to connect to the source controller.
	SourceCACert string `json:"source-ca-cert"`
	// CrossModelUUIDs are the UUIDs of models containing offers to which
	// consumers in the migrated model are related.
	CrossModelUUIDs []string `json:"cross-model-uuids"`
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
	UUID                   string            `json:"uuid"`
	Name                   string            `json:"name"`
	Qualifier              string            `json:"qualifier"`
	AgentVersion           semversion.Number `json:"agent-version"`
	ControllerAgentVersion semversion.Number `json:"controller-agent-version"`
	FacadeVersions         map[string][]int  `json:"facade-versions,omitempty"`
	ModelDescription       []byte            `json:"model-description,omitempty"`
}

// MigrationStatus reports the current status of a model migration.
type MigrationStatus struct {
	MigrationId string `json:"migration-id"`
	Attempt     int    `json:"attempt"`
	Phase       string `json:"phase"`

	// TODO(mjs): I'm not convinced these Source fields will get used.
	SourceAPIAddrs []string `json:"source-api-addrs"`
	SourceCACert   string   `json:"source-ca-cert"`

	TargetAPIAddrs []string `json:"target-api-addrs"`
	TargetCACert   string   `json:"target-ca-cert"`
}

// PhaseResults holds the phase of one or more model migrations.
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

// AdoptResourcesArgs holds the information required to ask the
// provider to update the controller tags for a model's
// resources.
type AdoptResourcesArgs struct {
	// ModelTag identifies the model that owns the resources.
	ModelTag string `json:"model-tag"`

	// SourceControllerVersion indicates the version of the calling
	// controller. This is needed in case the way the resources are
	// tagged has changed between versions - the provider should
	// ensure it looks for the original tags in the correct format for
	// that version.
	SourceControllerVersion semversion.Number `json:"source-controller-version"`
}
