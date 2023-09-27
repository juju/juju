// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "database/sql"

// info holds the information about database upgrade
type info struct {
	// UUID holds the upgrader's ID
	UUID string `db:"uuid"`
	// PreviousVersion holds the previous version
	PreviousVersion string `db:"previous_version"`
	// TargetVersion holds the target version
	TargetVersion string `db:"target_version"`
	// CreatedAt holds the time the upgrade was created
	CreatedAt string `db:"created_at"`
	// StartedAt holds the time the upgrade was started
	StartedAt sql.NullString `db:"started_at"`
	// CompletedAt holds the time the upgrade was completed
	CompletedAt sql.NullString `db:"completed_at"`
	// DBCompletedAt holds the time the upgrade was completed in the DB
	DBCompletedAt sql.NullString `db:"db_completed_at"`
}

// infoControllerNode holds the information about completeness of database upgrade process for a particular controller node
type infoControllerNode struct {
	// ControllerNodeID holds the controller node ID
	ControllerNodeID string `db:"controller_node_id"`
	// NodeUpgradeStartedAt holds the time the upgrade was started on the node
	NodeUpgradeCompletedAt sql.NullString `db:"node_upgrade_completed_at"`
}
