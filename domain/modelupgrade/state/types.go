// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent model information in the database.
type dbModel struct {
	IsControllerModel bool `db:"is_controller_model"`
}

type dbAgentVersion struct {
	TargetVersion string `db:"target_version"`
}

type dbModelConfig struct {
	Value string `db:"value"`
}
