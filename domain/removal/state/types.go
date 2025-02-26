// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// entityUUID holds a UUID in string form.
type entityUUID struct {
	// UUID uniquely identifies a domain entity.
	UUID string `db:"uuid"`
}
