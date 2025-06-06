// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// controllerNodeAgentVersion represents the agent version running for each
// controller node in the cluster.
type controllerNodeAgentVersion struct {
	ControllerID string `db:"controller_id"`
	Version      string `db:"version"`
}

// controllerTargetVersion represents the current target version set for the
// controller.
type controllerTargetVersion struct {
	TargetVersion string `db:"target_version"`
}

// setControllerTargetVersion is the values required for setting the target
// controller version of the cluster.
type setControllerTargetVersion struct {
	TargetVersion string `db:"target_version"`
}
