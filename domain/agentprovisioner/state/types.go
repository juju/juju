// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/core/model"

// modelConfigRow represents a single key-value pair in model config.
type modelConfigRow struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// modelConfigRow represents a row from the read-only model table.
type modelInfo struct {
	ID model.UUID `db:"uuid"`
}
