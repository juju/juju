// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/permission"
)

// UserAccessSpec defines the attributes that can be set when adding a new
// user access.
type UserAccessSpec struct {
	User   string
	Target permission.ID
	Access permission.Access
}

// UpsertPermissionArgs defines the attributes necessary to
// Grant or Revoke permissions of a Subject (user) on a Target where
// a new user may be created to satisfy granting of permissions.
type UpsertPermissionArgs struct {
	// Access is what the permission access should change to.
	Access permission.Access
	// AddUser will add the subject if the user does not exist.
	AddUser bool
	// ApiUser is the user requesting the change, they must have
	// permission to do it as well.
	ApiUser string
	// What type of change to access is needed, grant or revoke?
	Change permission.AccessChange
	// Subject is the subject of the permission, e.g. user.
	Subject string
	// Target is the thing the subject's permission to is being
	// updated on.
	Target permission.ID
}
