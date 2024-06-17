// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// dbFlag represents a flag to be serialised to the database.
type dbFlag struct {
	Name        string `db:"name"`
	Value       bool   `db:"value"`
	Description string `db:"description"`
}
