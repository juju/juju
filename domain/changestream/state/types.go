// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "time"

// ModelNamespace represents a model and the associated DQlite namespace that it
// uses.
type ModelNamespace struct {
	Namespace string `db:"namespace"`
}

// Watermark represents a row from the change_log_witness table.
type Watermark struct {
	ControllerID string    `db:"controller_id"`
	LowerBound   int64     `db:"lower_bound"`
	UpdatedAt    time.Time `db:"updated_at"`
}
