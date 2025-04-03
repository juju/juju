// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/password"
)

// unitName represents a unit's name.
type unitName struct {
	UUID unit.UUID `db:"uuid"`
	Name string    `db:"name"`
}

// unitPasswordHash represents a unit's password.
type unitPasswordHash struct {
	UUID         unit.UUID             `db:"uuid"`
	PasswordHash password.PasswordHash `db:"password_hash"`
}

// validateUnitPasswordHash represents a unit's password.
type validateUnitPasswordHash struct {
	UUID         unit.UUID             `db:"uuid"`
	PasswordHash password.PasswordHash `db:"password_hash"`
	Count        int                   `db:"count"`
}

type unitPasswordHashes struct {
	UnitName     unit.Name             `db:"unit_name"`
	PasswordHash password.PasswordHash `db:"password_hash"`
}
