// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access

import (
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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
		return errors.Errorf("empty subject %w", coreerrors.NotValid)
	}
	if err := args.AccessSpec.Validate(); err != nil {
		return errors.Capture(err)
	}
	if args.Change != permission.Grant && args.Change != permission.Revoke {
		return errors.Errorf("change %q %w", args.Change, coreerrors.NotValid)
	}
	return nil
}

// OwnerModelAccess describes the owner's access level on a single model
// associated with one of their cloud credentials.
type OwnerModelAccess struct {
	// ModelName is the name of the model.
	ModelName string
	// ModelQualifier disambiguates models that share the same name across
	// different owners or namespaces.
	ModelQualifier model.Qualifier
	// OwnerAccess is the permission level the credential owner holds on
	// this model.
	OwnerAccess permission.Access
}

// OwnerModelAccessByCredential groups a credential key with all models the
// credential owner can access through that credential.
type OwnerModelAccessByCredential struct {
	// CredentialKey identifies the cloud credential.
	CredentialKey credential.Key
	// Models is the list of models accessible to the owner via this
	// credential, together with the owner's access level on each.
	Models []OwnerModelAccess
}

// OfferImport contains details to import access to an offer.
type OfferImportAccess struct {
	UUID   uuid.UUID
	Access map[string]permission.Access
}
