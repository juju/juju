// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import (
	"time"

	"github.com/juju/names/v4"
)

// UserAccess represents a user access to a target whereas the user
// could represent a remote user or a user across multiple models the
// user access always represents a single user for a single target.
// There should be no more than one UserAccess per target/user pair.
// Many of these fields are storage artifacts but generate them from
// other fields implies out of band knowledge of other packages.
type UserAccess struct {
	// UserID is the stored ID of the user.
	UserID string
	// UserTag is the tag for the user.
	UserTag names.UserTag
	// Object is the tag for the object of this access grant.
	Object names.Tag
	// Access represents the level of access subject has over object.
	Access Access
	// CreatedBy is the tag of the user that granted the access.
	CreatedBy names.UserTag
	// DateCreated is the date the user was created in UTC.
	DateCreated time.Time
	// DisplayName is the name we are showing for this user.
	DisplayName string
	// UserName is the actual username for this access.
	UserName string
}

// IsEmptyUserAccess returns true if the passed UserAccess instance
// is empty.
func IsEmptyUserAccess(a UserAccess) bool {
	return a == UserAccess{}
}
