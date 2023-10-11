// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/juju/core/upgrade"
	domainupgrade "github.com/juju/juju/domain/upgrade"
)

// info holds the information about database upgrade
type info struct {
	// UUID holds the upgrader's ID
	UUID domainupgrade.UUID `db:"uuid"`
	// PreviousVersion holds the previous version
	PreviousVersion string `db:"previous_version"`
	// TargetVersion holds the target version
	TargetVersion string `db:"target_version"`
	// State holds the current state of the upgrade.
	State upgrade.State `db:"state"`
}

// ToUpgradeInfo converts an info to an upgrade.Info.
func (i info) ToUpgradeInfo() (upgrade.Info, error) {
	result := upgrade.Info{
		UUID:            i.UUID.String(),
		PreviousVersion: i.PreviousVersion,
		TargetVersion:   i.TargetVersion,
		State:           i.State,
	}
	return result, nil
}

// infoControllerNode holds the information about completeness of database upgrade process for a particular controller node
type infoControllerNode struct {
	// ControllerNodeID holds the controller node ID
	ControllerNodeID string `db:"controller_node_id"`
	// NodeUpgradeStartedAt holds the time the upgrade was started on the node
	NodeUpgradeCompletedAt sql.NullString `db:"node_upgrade_completed_at"`
}
