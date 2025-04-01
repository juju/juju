// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import (
	"time"

	"github.com/juju/juju/core/user"
)

// UserAccess represents a user's access level for a target object.
// Whereas the user could represent a remote user or a user across multiple
// models, UserAccess always represents a single user access for a single target.
// There should be no more than one UserAccess per user/target pair.
type UserAccess struct {
	// UserID is the stored ID of the user.
	UserID string
	// PermissionID is the stored ID of the permission.
	PermissionID string
	// Object is the ID of the object of this access grant.
	Object ID
	// Access represents the level of access subject has over object.
	Access Access
	// CreatedBy is the tag of the user that granted the access.
	CreatedBy user.Name
	// DateCreated is the date the user was created in UTC.
	DateCreated time.Time
	// DisplayName is the name we are showing for this user.
	DisplayName string
	// UserName is the actual username for this access.
	UserName user.Name
}

// IsEmptyUserAccess returns true if the passed UserAccess instance
// is empty.
func IsEmptyUserAccess(a UserAccess) bool {
	return a == UserAccess{}
}
