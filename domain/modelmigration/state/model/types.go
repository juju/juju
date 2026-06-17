// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

// modelInfo represents the model's read only information from the model table
// in the model database.
type modelInfo struct {
	// ControllerUUID is the controllers unique id.
	ControllerUUID string `db:"controller_uuid"`
}

// modelType represents the model's deployment type.
type modelType struct {
	Type string `db:"type"`
}

// agentName represents an agent-bearing entity name.
type agentName struct {
	Name string `db:"name"`
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

// offererModel represents a distinct (offerer controller, offerer model) pair
// referenced by the model's remote applications.
type offererModel struct {
	ControllerUUID string `db:"offerer_controller_uuid"`
	ModelUUID      string `db:"offerer_model_uuid"`
}

// setAgentVersionTarget represents the set of update values required for
// setting the model's target agent version.
type setAgentVersionTarget struct {
	TargetVersion   string `db:"target_version"`
	PreviousVersion string `db:"previous_version"`
}
