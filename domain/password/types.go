// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package password

import "github.com/juju/juju/core/unit"

// PasswordHash represents a hashed password.
type PasswordHash string

func (p PasswordHash) String() string {
	return string(p)
}

// UnitPasswordHashes represents a map of unit names to password hashes.
type UnitPasswordHashes map[unit.Name]PasswordHash
