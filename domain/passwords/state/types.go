// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/passwords"
)

// unitName represents a unit's name.
type unitName struct {
	UUID unit.UUID `db:"uuid"`
	Name string    `db:"name"`
}

// unitPasswordHash represents a unit's password.
type unitPasswordHash struct {
	UUID         unit.UUID              `db:"uuid"`
	PasswordHash passwords.PasswordHash `db:"password_hash"`
}

type unitPasswordHashes struct {
	ApplicationName string                 `db:"application_name"`
	UnitName        unit.Name              `db:"unit_name"`
	PasswordHash    passwords.PasswordHash `db:"password_hash"`
}
