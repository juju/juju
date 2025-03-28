// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/errors"
)

// EveryoneUserName represents a special user that is has the base permission
// level of all external users.
var EveryoneUserName, _ = user.NewName("everyone@external")

// AccessSpec defines the attributes that can be set when adding a new
// access.
type AccessSpec struct {
	Target ID
	Access Access
}

// Validate validates that the access and target specified in the
// spec are values allowed together and that the User is not an
// empty string. If any of these are untrue, a NotValid error is
// returned.
func (u AccessSpec) Validate() error {
	if err := u.Target.Validate(); err != nil {
		return err
	}
	if err := u.Target.ValidateAccess(u.Access); err != nil {
		return err
	}
	return nil
}

// RevokeAccess returns the new access level based on the revoking the current
// value setting. E.g. revoking SuperuserAccess sets LoginAccess for
// controllers.
func (a AccessSpec) RevokeAccess() Access {
	switch a.Target.ObjectType {
	case Cloud:
		return cloudRevoke(a.Access)
	case Controller:
		return controllerRevoke(a.Access)
	case Model:
		return modelRevoke(a.Access)
	case Offer:
		return offerRevoke(a.Access)
	default:
		return NoAccess
	}
}

// UserAccessSpec defines the attributes that can be set when adding a new
// user access.
type UserAccessSpec struct {
	AccessSpec
	User user.Name
}

// Validate validates that the access and target specified in the
// spec are values allowed together and that the User is not an
// empty string. If any of these are untrue, a NotValid error is
// returned.
func (u UserAccessSpec) Validate() error {
	if u.User.IsZero() {
		return errors.Errorf("empty user %w", coreerrors.NotValid)
	}
	if err := u.AccessSpec.Validate(); err != nil {
		return err
	}
	return nil
}
