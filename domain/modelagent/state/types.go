// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// dbAgentVersion represents the target agent version from the model table.
type dbAgentVersion struct {
	TargetAgentVersion string `db:"target_version"`
}

// machineName represents the single column of a machine that is the machines
// name.
type machineName struct {
	Name string `db:"name"`
}

// modelUUIDValue represents a model id for associating public keys with.
type modelUUIDValue struct {
	UUID string `db:"model_uuid"`
}

// unitName represents the single column of a unit that is the unit's name.
type unitName struct {
	Name string `db:"name"`
}
