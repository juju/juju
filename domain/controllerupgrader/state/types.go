// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// agentStoreBinary represents an agent binary that exists within the
// object store.
type agentStoreBinary struct {
	ArchitectureID int    `db:"architecture_id"`
	StreamID       int    `db:"stream_id"`
	Version        string `db:"version"`
}

// controllerNodeAgentVersion represents the agent version running for each
// controller node in the cluster.
type controllerNodeAgentVersion struct {
	ControllerID string `db:"controller_id"`
	Version      string `db:"version"`
}

// agentVersionTarget represents the target agent version column from the
// agent_version table.
type agentVersionTarget struct {
	TargetVersion string `db:"target_version"`
}

// controllerTargetVersion represents the current target version set for the
// controller.
type controllerTargetVersion struct {
	TargetVersion string `db:"target_version"`
}

// isControllerModel represents the is_controller_model column value from  the
// model table in the controller's model database.
type isControllerModel struct {
	Is bool `db:"is_controller_model"`
}

// setAgentVersionTarget represents the set of update values required for
// setting the model's target agent version.
type setAgentVersionTarget struct {
	TargetVersion string `db:"target_version"`
}

// setAgentVersionTargetStream represents the set of update values required for
// setting the model's target agent version and stream.
type setAgentVersionTargetStream struct {
	StreamID      int    `db:"stream_id"`
	TargetVersion string `db:"target_version"`
}

// setControllerTargetVersion is the values required for setting the target
// controller version of the cluster.
type setControllerTargetVersion struct {
	TargetVersion string `db:"target_version"`
}

type ids []int

type binaryVersion struct {
	Version string `db:"version"`
}

// binaryForVersionAndArchitectures represents the binary agent version
// we want to check the existence for.
type binaryForVersionAndArchitectures struct {
	ArchitectureID int    `db:"architecture_id"`
	Version        string `db:"version"`
}

// agentStream represents the stream in use for the agent.
type agentStream struct {
	StreamID int `db:"stream_id"`
}
