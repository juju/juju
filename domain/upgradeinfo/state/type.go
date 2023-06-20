// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent the persistent upgrade info schema in the database.

type Info struct {
	UUID            string `db:"uuid"`
	PreviousVersion string `db:"previous_version"`
	TargetVersion   string `db:"target_version"`
	InitTime        string `db:"init_time"`
	StartTime       string `db:"start_time"`
	CompletionTime  string `db:"completion_time"`
}

type InfoControllerNode struct {
	ControllerNodeID string `db:"controller_node_id"`
	NodeStatus       string `db:"status"`
}
