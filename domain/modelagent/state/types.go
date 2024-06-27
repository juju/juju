// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// dbAgentVersion represents the target agent version from the model table.
type dbAgentVersion struct {
	TargetAgentVersion string `db:"target_agent_version"`
}
