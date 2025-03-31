// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package passwords

// PasswordHash represents a hashed password.
type PasswordHash string

func (p PasswordHash) String() string {
	return string(p)
}
