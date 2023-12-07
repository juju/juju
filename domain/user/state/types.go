// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/juju/core/user"
)

// User represents a user in the state layer with the associated fields in the database.
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

	// CreatedAt is the time that the user was created at.
	CreatedAt time.Time `db:"created_at"`
}

// toCoreUser converts the state user to a core user.
func (u User) toCoreUser() user.User {
	return user.User{
		UUID:        u.UUID,
		Name:        u.Name,
		DisplayName: u.DisplayName,
		CreatorUUID: u.CreatorUUID,
		CreatedAt:   u.CreatedAt,
	}
}
