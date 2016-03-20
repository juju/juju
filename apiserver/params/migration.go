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

// MigrationStatus reports the current status of a model migration.
type MigrationStatus struct {
	Attempt        int             `json:"attempt"`
	Phase          migration.Phase `json:"phase"`
	SourceAPIAddrs []string        `json:"source-api-addrs"`
	TargetAPIAddrs []string        `json:"target-api-addrs"`
}
