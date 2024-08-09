// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/core/model"

// dbAgentVersion represents the target agent version from the model table.
type dbAgentVersion struct {
	TargetAgentVersion string `db:"target_agent_version"`
}

// modelIDArgs defines the UUID for a model.
type modelIDArgs struct {
	ModelID model.UUID `db:"model_id"`
}

// modelNameArgs defines a fully-qualified model name (owner + model name).
type modelNameArgs struct {
	ModelName string `db:"model_name"`
	Owner     string `db:"owner"`
}
