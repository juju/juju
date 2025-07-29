// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import "github.com/juju/juju/domain/life"

// entityUUID holds a UUID in string form.
type entityUUID struct {
	// UUID uniquely identifies a domain entity.
	UUID string `db:"uuid"`
}

// entityLife holds an entity's life in integer
type entityLife struct {
	Life life.Life `db:"life_id"`
}
