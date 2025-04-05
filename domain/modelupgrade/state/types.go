// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent model information in the database.
type dbModelInfo struct {
	IsControllerModel bool   `db:"is_controller_model"`
	TargetVersion     string `db:"target_version"`
}
