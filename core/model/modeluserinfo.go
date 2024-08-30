// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"time"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
)

// ModelUserInfo contains information about a user of a model.
type ModelUserInfo struct {
	// Name is the username of the user.
	Name user.Name
	// DisplayName is a user-friendly name representation of the users name.
	DisplayName string
	// Access represents the level of model access this user has.
	Access permission.Access
	// LastLogin is the last time the user logged in.
	LastModelLogin time.Time
}
