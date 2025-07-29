// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import "github.com/juju/juju/domain/life"

// entityUUIDs is a slice of entityUUID, used to hold multiple UUIDs.
type uuids []string

// entityUUID holds a UUID in string form.
type entityUUID struct {
	// UUID uniquely identifies a domain entity.
	UUID string `db:"uuid"`
}

// entityAssociationCount holds a Count in int form and the UUID in string form
// for the associated entity.
type entityAssociationCount struct {
	// UUID uniquely identifies a associated domain entity.
	UUID string `db:"uuid"`
	// Count counts the number of entities.
	Count int `db:"count"`
}

type count struct {
	Count int `db:"count"`
}

// entityLife holds an entity's life in integer
type entityLife struct {
	Life life.Life `db:"life_id"`
}
