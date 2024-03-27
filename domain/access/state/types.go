// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/names/v5"

	corepermission "github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
)

// user represents a user in the state layer with the associated fields in the
// database.
type dbUser struct {
	// UUID is the unique identifier for the user.
	UUID coreuser.UUID `db:"uuid"`

	// Name is the username of the user.
	Name string `db:"name"`

	// DisplayName is a user-friendly name represent the user as.
	DisplayName string `db:"display_name"`

	// Removed indicates if the user has been removed.
	Removed bool `db:"removed"`

	// CreatorUUID is the associated user that created this user.
	CreatorUUID coreuser.UUID `db:"created_by_uuid"`

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
func (u dbUser) toCoreUser() coreuser.User {
	return coreuser.User{
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

// dbActivationKey represents an activation key in the state layer with the
// associated fields in the database.
type dbActivationKey struct {
	ActivationKey string `db:"activation_key"`
}

// dbPermissionUser represents a user in the system where the values overlap
// with corepermission.UserAccess.
type dbPermissionUser struct {
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
func (u dbPermissionUser) toCoreUserAccess() corepermission.UserAccess {
	return corepermission.UserAccess{
		UserID:      u.UUID,
		UserTag:     names.NewUserTag(u.Name),
		DisplayName: u.DisplayName,
		UserName:    u.Name,
		CreatedBy:   names.NewUserTag(u.CreatorName),
		DateCreated: u.CreatedAt,
	}
}

// dbAddUserPermission represents a permission in the system where the values
// overlap with corepermission.Permission.
type dbAddUserPermission struct {
	// UUID is the unique identifier for the permission.
	UUID string `db:"uuid"`

	// PermissionType is the type of permission.
	PermissionType int64 `db:"permission_type_id"`

	// GrantOn is the tag that the permission is granted on.
	GrantOn string `db:"grant_on"`

	// GrantTo is the tag that the permission is granted to.
	GrantTo string `db:"grant_to"`
}

// dbReadUserPermission represents a permission in the system.
type dbReadUserPermission struct {
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

// toUserAccess combines a dbReadUserPermission with a user to create
// a core permission UserAccess.
func (r dbReadUserPermission) toUserAccess(u dbPermissionUser) corepermission.UserAccess {
	userAccess := u.toCoreUserAccess()
	userAccess.PermissionID = r.UUID
	userAccess.Object = objectTag(corepermission.ID{
		ObjectType: corepermission.ObjectType(r.ObjectType),
		Key:        r.GrantOn,
	})
	userAccess.Access = corepermission.Access(r.AccessType)
	return userAccess
}
