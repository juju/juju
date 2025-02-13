// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
)

// UserAccessFunc represents a func that can answer the question about what
// level of access a user has for a given target.
type UserAccessFunc func(ctx context.Context, userName coreuser.Name, target permission.ID) (permission.Access, error)

// HasPermission returns true if the specified user has the specified
// permission on target.
func HasPermission(
	ctx context.Context,
	accessGetter UserAccessFunc,
	utag names.Tag,
	requestedPermission permission.Access,
	target names.Tag,
) (bool, error) {
	var objectType permission.ObjectType
	var validate func(permission.Access) error
	switch target.Kind() {
	case names.ControllerTagKind:
		objectType = permission.Controller
		validate = permission.ValidateControllerAccess
	case names.ModelTagKind:
		objectType = permission.Model
		validate = permission.ValidateModelAccess
	case names.ApplicationOfferTagKind:
		objectType = permission.Offer
		validate = permission.ValidateOfferAccess
	case names.CloudTagKind:
		objectType = permission.Cloud
		validate = permission.ValidateCloudAccess
	default:
		return false, nil
	}
	if err := validate(requestedPermission); err != nil {
		return false, nil
	}

	userTag, ok := utag.(names.UserTag)
	if !ok {
		// Reveal no more than is strictly necessary.
		return false, nil
	}

	userAccess, err := accessGetter(ctx, coreuser.NameFromTag(userTag), permission.ID{
		ObjectType: objectType,
		Key:        target.Id(),
	})
	if err != nil && !(errors.Is(err, accesserrors.AccessNotFound) || errors.Is(err, accesserrors.UserNotFound)) {
		return false, errors.Annotatef(err, "while obtaining %s user", target.Kind())
	}
	if userAccess == permission.NoAccess {
		return false, nil
	}

	modelPermission := userAccess.EqualOrGreaterModelAccessThan(requestedPermission) && target.Kind() == names.ModelTagKind
	controllerPermission := userAccess.EqualOrGreaterControllerAccessThan(requestedPermission) && target.Kind() == names.ControllerTagKind
	offerPermission := userAccess.EqualOrGreaterOfferAccessThan(requestedPermission) && target.Kind() == names.ApplicationOfferTagKind
	cloudPermission := userAccess.EqualOrGreaterCloudAccessThan(requestedPermission) && target.Kind() == names.CloudTagKind
	if !controllerPermission && !modelPermission && !offerPermission && !cloudPermission {
		return false, nil
	}
	return true, nil
}
