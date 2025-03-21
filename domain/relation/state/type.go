// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

type relationIDAndUUID struct {
	// UUID is the UUID of the relation.
	UUID string `db:"uuid"`
	// ID is the numeric ID of the relation
	ID int `db:"relation_id"`
}
