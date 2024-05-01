// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/permission"
)

// UpdatePermissionArgs are necessary arguments to run
// UpdatePermissionOnTarget.
type UpdatePermissionArgs struct {
	// AccessSpec is what the permission access should change to
	// combined with the target the subject's permission to is being
	// updated on.
	AccessSpec permission.AccessSpec
	// AddUser will add the subject if the user does not exist.
	AddUser bool
	// ApiUser is the user requesting the change, they must have
	// permission to do it as well.
	ApiUser string
	// What type of change to access is needed, grant or revoke?
	Change permission.AccessChange
	// Subject is the subject of the permission, e.g. user.
	Subject string
}

func (args UpdatePermissionArgs) Validate() error {
	if args.ApiUser == "" {
		return errors.Trace(errors.NotValidf("empty api user"))
	}
	if args.Subject == "" {
		return errors.Trace(errors.NotValidf("empty subject"))
	}
	if err := args.AccessSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if args.Change != permission.Grant && args.Change != permission.Revoke {
		return errors.Trace(errors.NotValidf("change %q", args.Change))
	}
	return nil
}

// ModelUserInfo holds information on a user who has access to a model. Owners
// of a model can see this information for all users who have access, so it
// should not include sensitive information.
type ModelUserInfo struct {
	// The users name.
	UserName string
	// The users display name.
	DisplayName string
	// The last time the user connected to the model.
	LastConnection *time.Time
	// The level of access the user has on the model.
	Access permission.Access
}
