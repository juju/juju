// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/upgrade"
)

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

// ToUpgradeInfo converts an info to an upgrade.Info.
func (i info) ToUpgradeInfo() (upgrade.Info, error) {
	result := upgrade.Info{
		UUID:            i.UUID,
		PreviousVersion: i.PreviousVersion,
		TargetVersion:   i.TargetVersion,
	}

	var err error

	if result.CreatedAt, err = time.Parse(time.RFC3339, i.CreatedAt); err != nil {
		return result, errors.Trace(err)
	}

	if i.StartedAt.Valid {
		if result.StartedAt, err = time.Parse(time.RFC3339, i.StartedAt.String); err != nil {
			return result, errors.Trace(err)
		}
	}
	if i.CompletedAt.Valid {
		if result.CompletedAt, err = time.Parse(time.RFC3339, i.CompletedAt.String); err != nil {
			return result, errors.Trace(err)
		}
	}
	if i.DBCompletedAt.Valid {
		if result.DBCompletedAt, err = time.Parse(time.RFC3339, i.DBCompletedAt.String); err != nil {
			return result, errors.Trace(err)
		}
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
