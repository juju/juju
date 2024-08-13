// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
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
	// External must be set if AddUser is true. It indicates if the subject is
	// an external or local user.
	External *bool
	// ApiUser is the user requesting the change, they must have
	// permission to do it as well.
	ApiUser user.Name
	// What type of change to access is needed, grant or revoke?
	Change permission.AccessChange
	// Subject is the subject of the permission, e.g. user.
	Subject user.Name
}

func (args UpdatePermissionArgs) Validate() error {
	if args.ApiUser.IsZero() {
		return errors.Trace(errors.NotValidf("apiUser name is zero"))
	}
	if args.Subject.IsZero() {
		return errors.Trace(errors.NotValidf("empty subject"))
	}
	if args.AddUser && (args.External == nil) {
		return errors.Trace(errors.NotValidf("add user is true but external is not set"))
	}
	if err := args.AccessSpec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if args.Change != permission.Grant && args.Change != permission.Revoke {
		return errors.Trace(errors.NotValidf("change %q", args.Change))
	}
	return nil
}

// CredentialOwnerModelAccess stores cloud credential model information for the credential owner
// or an error retrieving it.
type CredentialOwnerModelAccess struct {
	ModelName   string            `db:"model_name"`
	OwnerAccess permission.Access `db:"access_type"`
}

// ModelUserInfo contains information about a user of a model.
type ModelUserInfo struct {
	// Name is the username of the user.
	Name user.Name
	// DisplayName is a user-friendly name representation of the users name.
	DisplayName string
	// Access represents the level of model access this user has.
	Access permission.Access
	// LastLogin is the last time the user logged in.
	LastModelLogin time.Time
}
