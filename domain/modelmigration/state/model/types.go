// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

// modelInfo represents the model's read only information from the model table
// in the model database.
type modelInfo struct {
	// ControllerUUID is the controllers unique id.
	ControllerUUID string `db:"controller_uuid"`
}

// instanceID represents the struct to be used for the instance_id column within
// the sqlair statements in the machine domain.
type instanceID struct {
	ID string `db:"instance_id"`
}

type entityUUID struct {
	UUID string `db:"uuid"`
}

// agentVersionTarget represents the target agent version column from the
// agent_version table.
type agentVersionTarget struct {
	TargetVersion string `db:"target_version"`
}

// setAgentVersionTarget represents the set of update values required for
// setting the model's target agent version.
type setAgentVersionTarget struct {
	TargetVersion string `db:"target_version"`
}
