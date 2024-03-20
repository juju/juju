// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
)

// AddUserArg represents the arguments for creating a single user.
type AddUserArg struct {
	// UUID is an optional unique identifier for the user.
	// If it is empty, one will be generated during creation.
	UUID user.UUID

	// Name is the identifying name for the user.
	Name string

	// Display name is the user's short name for display.
	DisplayName string

	// Password is an optional password for the user.
	// If it is empty, a one-time key is generated for the user's first login.
	Password *auth.Password

	// CreatorUUID identifies the user that requested this creation.
	CreatorUUID user.UUID

	// Permissions are the permissions to grant to the user upon creation.
	// If no permission is passed, then NoAccess is set.
	Permission permission.UserPermissionAccess
}
