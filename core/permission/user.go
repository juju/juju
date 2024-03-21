// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/database"
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
			Key:        database.ControllerNS,
		},
	}
}
