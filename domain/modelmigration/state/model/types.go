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

type entityUUID struct {
	UUID string `db:"uuid"`
}

// machineInstanceID pairs a machine name with its provider cloud instance ID.
type machineInstanceID struct {
	MachineName string `db:"name"`
	InstanceID  string `db:"instance_id"`
}

// secretBackendUUID represents a secret backend UUID as referenced by a
// model-database secret value ref.
type secretBackendUUID struct {
	BackendUUID string `db:"backend_uuid"`
}

// revisionBackend pairs a secret revision UUID with the backend UUID its
// external value ref points at.
type revisionBackend struct {
	RevisionUUID string `db:"revision_uuid"`
	BackendUUID  string `db:"backend_uuid"`
}

// architectureName represents an architecture's name.
type architectureName struct {
	Name string `db:"name"`
}

// versionArg carries an agent binary version for use as a query argument.
type versionArg struct {
	Version string `db:"version"`
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
