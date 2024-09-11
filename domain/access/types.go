// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access

import (
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
	// What type of change to access is needed, grant or revoke?
	Change permission.AccessChange
	// Subject is the subject of the permission, e.g. user.
	Subject user.Name
}

func (args UpdatePermissionArgs) Validate() error {
	if args.Subject.IsZero() {
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

// CredentialOwnerModelAccess stores cloud credential model information for the credential owner
// or an error retrieving it.
type CredentialOwnerModelAccess struct {
	ModelName   string            `db:"model_name"`
	OwnerAccess permission.Access `db:"access_type"`
}
