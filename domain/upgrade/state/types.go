// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/internal/errors"
)

// Info holds the information about database upgrade
type Info struct {
	// UUID holds the upgrader's ID
	UUID string `db:"uuid"`
	// PreviousVersion holds the previous version
	PreviousVersion string `db:"previous_version"`
	// TargetVersion holds the target version
	TargetVersion string `db:"target_version"`
	// StateIDType holds the type id of the current state of the upgrade.
	StateIDType int `db:"state_type_id"`
}

// ToUpgradeInfo converts an info to an upgrade.Info.
func (i Info) ToUpgradeInfo() (upgrade.Info, error) {
	state := upgrade.State(i.StateIDType)
	if _, ok := upgrade.States[state]; !ok {
		return upgrade.Info{}, errors.Errorf("unknown state id %q", i)
	}
	result := upgrade.Info{
		UUID:            i.UUID,
		PreviousVersion: i.PreviousVersion,
		TargetVersion:   i.TargetVersion,
		State:           state,
	}
	return result, nil
}

// ControllerNodeInfo holds the information about completeness of database
// upgrade process for a particular controller node
type ControllerNodeInfo struct {
	// UUID holds the rows UUID.
	UUID string `db:"uuid"`
	// UpgradeInfoUUID holds the UUID of the associated upgrade info.
	UpgradeInfoUUID string `db:"upgrade_info_uuid"`
	// ControllerNodeID holds the controller node ID
	ControllerNodeID string `db:"controller_node_id"`
	// NodeUpgradeStartedAt holds the time the upgrade was started on the node
	NodeUpgradeCompletedAt sql.NullString `db:"node_upgrade_completed_at"`
}

// Count is used to select counts from the database.
type Count struct {
	Num int `db:"num"`
}
