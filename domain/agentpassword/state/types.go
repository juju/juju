// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/agentpassword"
)

// unitName represents a unit's name.
type entityName struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

type validatePasswordHash struct {
	UUID         string                     `db:"uuid"`
	PasswordHash agentpassword.PasswordHash `db:"password_hash"`
	Count        int                        `db:"count"`
}

type validatePasswordHashWithNonce struct {
	UUID         string                     `db:"uuid"`
	PasswordHash agentpassword.PasswordHash `db:"password_hash"`
	Nonce        string                     `db:"nonce"`
}

type entityPasswordHash struct {
	UUID         string                     `db:"uuid"`
	PasswordHash agentpassword.PasswordHash `db:"password_hash"`
}

type entityNamePasswordHashes struct {
	Name         string                     `db:"name"`
	PasswordHash agentpassword.PasswordHash `db:"password_hash"`
}

type unitPasswordHashes struct {
	UnitName     unit.Name                  `db:"unit_name"`
	PasswordHash agentpassword.PasswordHash `db:"password_hash"`
}

type count struct {
	Count int `db:"count"`
}

type machineName struct {
	Name machine.Name `db:"name"`
}

type machineUUID struct {
	UUID machine.UUID `db:"uuid"`
}
