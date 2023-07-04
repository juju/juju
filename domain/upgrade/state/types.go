// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "database/sql"

type info struct {
	UUID            string         `db:"uuid"`
	PreviousVersion string         `db:"previous_version"`
	TargetVersion   string         `db:"target_version"`
	CreatedAt       string         `db:"created_at"`
	StartedAt       sql.NullString `db:"started_at"`
	CompletedAt     sql.NullString `db:"completed_at"`
}

type infoControllerNode struct {
	ControllerNodeID       string         `db:"controller_node_id"`
	NodeUpgradeCompletedAt sql.NullString `db:"node_upgrade_completed_at"`
}
