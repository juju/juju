// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/juju/core/user"
)

// User represents a user in the state layer with the associated fields in the
// database.
type User struct {
	// UUID is the unique identifier for the user.
	UUID user.UUID `db:"uuid"`

	// Name is the username of the user.
	Name string `db:"name"`

	// DisplayName is a user-friendly name represent the user as.
	DisplayName string `db:"display_name"`

	// Removed indicates if the user has been removed.
	Removed bool `db:"removed"`

	// CreatorUUID is the associated user that created this user.
	CreatorUUID user.UUID `db:"created_by_uuid"`

	// CreatorName is the name of the user that created this user.
	CreatorName string `db:"created_by_name"`

	// CreatedAt is the time that the user was created at.
	CreatedAt time.Time `db:"created_at"`

	// LastLogin is the last time the user logged in.
	LastLogin time.Time `db:"last_login"`

	// Disabled is true if the user is disabled.
	Disabled bool `db:"disabled"`

	// PasswordHash is the hash of the password.
	PasswordHash string `db:"password_hash"`

	// PasswordSalt is the salt used to hash the password.
	PasswordSalt []byte `db:"password_salt"`
}

// toCoreUser converts the state user to a core user.
func (u User) toCoreUser() user.User {
	return user.User{
		UUID:        u.UUID,
		Name:        u.Name,
		DisplayName: u.DisplayName,
		CreatorUUID: u.CreatorUUID,
		CreatorName: u.CreatorName,
		CreatedAt:   u.CreatedAt,
		LastLogin:   u.LastLogin,
		Disabled:    u.Disabled,
	}
}

// ActivationKey represents an activation key in the state layer with the
// associated fields in the database.
type ActivationKey struct {
	ActivationKey string `db:"activation_key"`
}
