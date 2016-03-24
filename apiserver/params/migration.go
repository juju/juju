// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import "github.com/juju/juju/core/migration"

// SetMigrationPhaseArgs provides a migration phase to the
// migrationmaster.SetPhase API method.
type SetMigrationPhaseArgs struct {
	Phase string `json:"phase"`
}

// SerializedModel wraps a buffer contain a serialised Juju model.
type SerializedModel struct {
	Bytes []byte `json:"bytes"`
}

// ModelArgs wraps a simple model tag.
type ModelArgs struct {
	ModelTag string `json:"model-tag"`
}

// MigrationStatus reports the current status of a model migration.
type MigrationStatus struct {
	Attempt int `json:"attempt"`
	// TODO(fwereade): shouldn't Phase be a string?
	Phase          migration.Phase `json:"phase"`
	SourceAPIAddrs []string        `json:"source-api-addrs"`
	SourceCACert   string          `json:"source-ca-cert"`
	TargetAPIAddrs []string        `json:"target-api-addrs"`
	TargetCACert   string          `json:"target-ca-cert"`
}

type PhaseResult struct {
	Phase string `json:"phase"`
	Error *Error
}

type PhaseResults struct {
	Results []PhaseResult `json:"Results"`
}
