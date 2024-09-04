// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// unitUUID identifies a unit.
type unitUUID struct {
	// UUID is the universally unique identifier for a unit.
	UUID string `db:"uuid"`
}

// unitName identifies a unit.
type unitName struct {
	// Name uniquely identifies a unit and indicates its application.
	// For example, postgresql/3.
	Name string `db:"name"`
}
