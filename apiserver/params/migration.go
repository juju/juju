// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// SetMigrationPhaseArgs provides a migration phase to the
// migrationmaster.SetPhase API method.
type SetMigrationPhaseArgs struct {
	Phase string `json:"phase"`
}

// SerializedModel wraps a buffer contain a serialised Juju model.
type SerializedModel struct {
	Bytes []byte `json:"bytes"`
}
