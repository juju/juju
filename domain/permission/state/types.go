// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/names/v5"

	corepermission "github.com/juju/juju/core/permission"
)

// User represents a user in the system where the values overlap with
// corepermission.UserAccess.
type user struct {
	// UUID is the unique identifier for the user.
	UUID string `db:"uuid"`

	// Name is the username of the user.
	Name string `db:"name"`

	// DisplayName is a user-friendly name represent the user as.
	DisplayName string `db:"display_name"`

	// CreatorName is the name of the user that created this user.
	CreatorName string `db:"created_by_name"`

	// CreatedAt is the time that the user was created at.
	CreatedAt time.Time `db:"created_at"`

	// Disabled is true if the user is disabled.
	Disabled bool `db:"disabled"`
}

// toCoreUserAccess converts the state user to a core permission UserAccess.
// Additional detail regarding the permission is required to be added
// after.
func (u user) toCoreUserAccess() corepermission.UserAccess {
	return corepermission.UserAccess{
		UserID:      u.UUID,
		UserTag:     names.NewUserTag(u.Name),
		DisplayName: u.DisplayName,
		UserName:    u.Name,
		CreatedBy:   names.NewUserTag(u.CreatorName),
		DateCreated: u.CreatedAt,
	}
}

// addUserPermission represents a permission in the system where the values
// overlap with corepermission.Permission.
type addUserPermission struct {
	// UUID is the unique identifier for the permission.
	UUID string `db:"uuid"`

	// PermissionType is the type of permission.
	PermissionType int64 `db:"permission_type_id"`

	// GrantOn is the tag that the permission is granted on.
	GrantOn string `db:"grant_on"`

	// GrantTo is the tag that the permission is granted to.
	GrantTo string `db:"grant_to"`
}

// readUserPermission represents a permission in the system.
type readUserPermission struct {
	// UUID is the unique identifier for the permission.
	UUID string `db:"uuid"`

	// GrantOn is the unique identifier of the permission target.
	// A name or UUID depending on the ObjectType.
	GrantOn string `db:"grant_on"`

	// GrantTo is the unique identifier of the user the permission
	// is granted to.
	GrantTo string `db:"grant_to"`

	// AccessType is a string version of core permission AccessType.
	AccessType string `db:"access_type"`

	// ObjectType is a string version of core permission ObjectType.
	ObjectType string `db:"object_type"`
}

// toUserAccess combines a readUserPermission with a user to create
// a core permission UserAccess.
func (r readUserPermission) toUserAccess(u user) corepermission.UserAccess {
	userAccess := u.toCoreUserAccess()
	userAccess.PermissionID = r.UUID
	userAccess.Object = objectTag(corepermission.ID{
		ObjectType: corepermission.ObjectType(r.ObjectType),
		Key:        r.GrantOn,
	})
	userAccess.Access = corepermission.Access(r.AccessType)
	return userAccess
}
