// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"time"
)

// User describes a user within Juju.
type User struct {
	// CreatedAt is the time that the user was created at.
	CreatedAt time.Time

	// DisplayName is a user friendly name represent the user as.
	DisplayName string

	// Name is the username of the user.
	Name string

	// Creator is the username of the user that created this user.
	Creator string
}
