// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import (
	"github.com/juju/errors"
)

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
	User string
}

// Validate validates that the access and target specified in the
// spec are values allowed together and that the User is not an
// empty string. If any of these are untrue, a NotValid error is
// returned.
func (u UserAccessSpec) Validate() error {
	if u.User == "" {
		return errors.NotValidf("empty user")
	}
	if err := u.AccessSpec.Validate(); err != nil {
		return err
	}
	return nil
}

// ControllerForAccess is the access spec for the controller
// login access.
func ControllerForAccess(access Access) AccessSpec {
	return AccessSpec{
		Access: access,
		Target: ID{
			ObjectType: Controller,
			// This should be controllerNS from the core/database package, but
			// using that import will cause the whole of the core/database
			// package into the api client package.
			// For now I've created a test to ensure that the value is correct.
			// TODO (stickupkid): Move controllerNS to a namespace package.
			Key: "controller",
		},
	}
}
