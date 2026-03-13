// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

// entityUUID holds a UUID in string form.
type entityUUID struct {
	// UUID uniquely identifies a domain entity.
	UUID string `db:"uuid"`
}

// entityLife holds an entity's life in integer
type entityLife struct {
	Life int `db:"life_id"`
}

type count struct {
	Count int `db:"count"`
}

type entityName struct {
	Name string `db:"name"`
}

type backendName struct {
	Name string `db:"name"`
}
