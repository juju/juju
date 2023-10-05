// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import "time"

// Info holds the information about database upgrade
type Info struct {
	// UUID holds the upgrader's ID
	UUID string
	// PreviousVersion holds the previous version
	PreviousVersion string
	// TargetVersion holds the target version
	TargetVersion string
	// CreatedAt holds the time the upgrade was created
	CreatedAt time.Time
	// StartedAt holds the time the upgrade was started
	StartedAt time.Time
	// DBCompletedAt holds the time the upgrade was completed in the DB
	DBCompletedAt time.Time
	// CompletedAt holds the time the upgrade was completed
	CompletedAt time.Time
}
