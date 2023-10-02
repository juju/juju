// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/upgrade"
)

type info struct {
	UUID            string         `db:"uuid"`
	PreviousVersion string         `db:"previous_version"`
	TargetVersion   string         `db:"target_version"`
	CreatedAt       string         `db:"created_at"`
	StartedAt       sql.NullString `db:"started_at"`
	CompletedAt     sql.NullString `db:"completed_at"`
}

// ToUpgradeInfo converts an info to an upgrade.Info.
func (i info) ToUpgradeInfo() (upgrade.Info, error) {
	result := upgrade.Info{
		UUID:            i.UUID,
		PreviousVersion: i.PreviousVersion,
		TargetVersion:   i.TargetVersion,
	}

	var err error

	if result.CreatedAt, err = time.Parse("whatever the layout is", i.CreatedAt); err != nil {
		return result, errors.Trace(err)
	}

	// Repeat for other nullable dates.
	if i.StartedAt.Valid {
		if result.StartedAt, err = time.Parse("whatever the layout is", i.StartedAt.String); err != nil {
			return result, errors.Trace(err)
		}
	}

	return result, nil
}

type infoControllerNode struct {
	ControllerNodeID       string         `db:"controller_node_id"`
	NodeUpgradeCompletedAt sql.NullString `db:"node_upgrade_completed_at"`
}
