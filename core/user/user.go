// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"time"
)

type User struct {
	// CreatedAt is the time that the user was created at.
	CreatedAt time.Time

	// DisplayName is a user-friendly name represent the user as.
	DisplayName string

	// Name is the username of the user.
	Name string

	// CreatorUUID is the associated user that created this user.
	CreatorUUID string
}
