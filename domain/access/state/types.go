// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	corepermission "github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
)

// user represents a user in the state layer with the associated fields in the
// database.
type dbUser struct {
	// UUID is the unique identifier for the user.
	UUID string `db:"uuid"`

	// Name is the username of the user.
	Name string `db:"name"`

	// DisplayName is a user-friendly name represent the user as.
	DisplayName string `db:"display_name"`

	// External indicates if the user has a non-local domain.
	External bool `db:"external"`

	// Removed indicates if the user has been removed.
	Removed bool `db:"removed"`

	// CreatorUUID is the associated user that created this user.
	CreatorUUID string `db:"created_by_uuid"`

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
func (u dbUser) toCoreUser() (coreuser.User, error) {
	name, err := coreuser.NewName(u.Name)
	if err != nil {
		return coreuser.User{}, errors.Annotate(err, "user name from db")
	}
	var creatorName coreuser.Name
	if u.CreatorName != "" {
		creatorName, err = coreuser.NewName(u.CreatorName)
		if err != nil {
			return coreuser.User{}, errors.Annotate(err, "creator name from db")
		}
	}
	return coreuser.User{
		UUID:        coreuser.UUID(u.UUID),
		Name:        name,
		DisplayName: u.DisplayName,
		CreatorUUID: coreuser.UUID(u.CreatorUUID),
		CreatorName: creatorName,
		CreatedAt:   u.CreatedAt,
		LastLogin:   u.LastLogin,
		Disabled:    u.Disabled,
	}, nil
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

	// External indicates if the user is not a local user.
	External bool `db:"external"`

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
func (u dbPermissionUser) toCoreUserAccess() (corepermission.UserAccess, error) {
	name, err := coreuser.NewName(u.Name)
	if err != nil {
		return corepermission.UserAccess{}, errors.Trace(err)
	}

	return corepermission.UserAccess{
		UserID:      u.UUID,
		UserTag:     names.NewUserTag(u.Name),
		DisplayName: u.DisplayName,
		UserName:    name,
		CreatedBy:   names.NewUserTag(u.CreatorName),
		DateCreated: u.CreatedAt,
	}, nil
}

// dbPermission represents a permission in the system.
type dbPermission struct {
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

// toUserAccess combines a dbPermission with a user to create
// a core permission UserAccess.
func (r dbPermission) toUserAccess(u dbPermissionUser) (corepermission.UserAccess, error) {
	userAccess, err := u.toCoreUserAccess()
	if err != nil {
		return corepermission.UserAccess{}, errors.Trace(err)
	}

	userAccess.PermissionID = r.UUID
	userAccess.Object = objectTag(corepermission.ID{
		ObjectType: corepermission.ObjectType(r.ObjectType),
		Key:        r.GrantOn,
	})
	userAccess.Access = corepermission.Access(r.AccessType)
	return userAccess, nil
}

// userName is used to pass a user's name as an argument to SQL.
type userName struct {
	Name string `db:"name"`
}

// userUUID is used to retrieve the UUID from the user table.
type userUUID struct {
	UUID string `db:"uuid"`
}

// userIdentifier represents the unique information for a user that is used to
// identify them. This is useful when wanting to make a mapping between user
// name and id.
type userIdentifier struct {
	// UUID holds the unique id for the given user.
	UUID string `db:"uuid"`

	// Name is the unique user name.
	Name string `db:"name"`
}

// permInOut is a struct to replace sqlair.M with permission
// SQL that contains a user name, grant_on and access both
// input and output.
type permInOut struct {
	Name    string `db:"name"`
	GrantOn string `db:"grant_on"`
	Access  string `db:"access_type"`
}

// dbModelLastLogin is a struct used to record a users logging in to a particular
// model.
type dbModelLastLogin struct {
	UserUUID  string    `db:"user_uuid"`
	ModelUUID string    `db:"model_uuid"`
	Time      time.Time `db:"time"`
}

// dbModelUUID is a struct used to record a model UUID from the database.
type dbModelUUID struct {
	UUID string `db:"uuid"`
}

// dbModelExists is used to record if a row in the database exists by selecting true
// into it.
type dbModelExists struct {
	Exists bool `db:"exists"`
}

// dbEveryoneExternal represents the permissions of the everyone@external user.
type dbEveryoneExternal dbPermission
