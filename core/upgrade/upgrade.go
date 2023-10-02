// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import "database/sql"

type Info struct {
	UUID            string
	PreviousVersion string
	TargetVersion   string
	CreatedAt       string
	StartedAt       sql.NullString
	CompletedAt     sql.NullString
}
