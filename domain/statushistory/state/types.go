// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// readOnlyModel represents a read-only model for the status history.
type readOnlyModel struct {
	UUID  string `db:"uuid"`
	Name  string `db:"name"`
	Owner string `db:"owner"`
}
