// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import "time"

type Info struct {
	UUID            string
	PreviousVersion string
	TargetVersion   string
	CreatedAt       time.Time
	StartedAt       time.Time
	CompletedAt     time.Time
}
