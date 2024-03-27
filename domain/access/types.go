// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/permission"
)

// UpsertPermissionArgs are necessary arguments to run
// UpdatePermissionOnTarget.
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

func (args UpsertPermissionArgs) Validate() error {
	if args.ApiUser == "" {
		return errors.Trace(errors.NotValidf("empty api user"))
	}
	if args.Subject == "" {
		return errors.Trace(errors.NotValidf("empty subject"))
	}
	if err := args.Target.ValidateAccess(args.Access); err != nil {
		return errors.Trace(err)
	}
	if args.Change != permission.Grant && args.Change != permission.Revoke {
		return errors.Trace(errors.NotValidf("change %q", args.Change))
	}
	return nil
}
