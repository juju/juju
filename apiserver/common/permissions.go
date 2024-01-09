// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
)

// EveryoneTagName represents a special group that encompasses
// all external users.
const EveryoneTagName = "everyone@external"

// UserAccessFunc represents a func that can answer the question about what
// level of access a user entity has for a given subject tag.
type UserAccessFunc func(coreuser.User, names.UserTag, names.Tag) (permission.Access, error)

// HasPermission returns true if the specified user has the specified
// permission on target.
func HasPermission(
	usr coreuser.User,
	accessGetter UserAccessFunc, utag names.Tag,
	requestedPermission permission.Access, target names.Tag,
) (bool, error) {
	var validate func(permission.Access) error
	switch target.Kind() {
	case names.ControllerTagKind:
		validate = permission.ValidateControllerAccess
	case names.ModelTagKind:
		validate = permission.ValidateModelAccess
	case names.ApplicationOfferTagKind:
		validate = permission.ValidateOfferAccess
	case names.CloudTagKind:
		validate = permission.ValidateCloudAccess
	default:
		return false, nil
	}
	if err := validate(requestedPermission); err != nil {
		return false, nil
	}

	userTag, ok := utag.(names.UserTag)
	if !ok {
		// lets not reveal more than is strictly necessary
		return false, nil
	}

	userAccess, err := GetPermission(usr, accessGetter, userTag, target)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return false, errors.Annotatef(err, "while obtaining %s user", target.Kind())
	}
	// returning this kind of information would be too much information to reveal too.
	if errors.Is(err, errors.NotFound) || userAccess == permission.NoAccess {
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

// GetPermission returns the permission a user has on the specified target.
func GetPermission(usr coreuser.User, accessGetter UserAccessFunc, userTag names.UserTag, target names.Tag) (permission.Access, error) {
	userAccess, err := accessGetter(usr, userTag, target)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return permission.NoAccess, errors.Annotatef(err, "while obtaining %s user", target.Kind())
	}
	// there is a special case for external users, a group called everyone@external
	if !userTag.IsLocal() {
		// TODO(perrito666) remove the following section about everyone group
		// when groups are implemented.
		everyoneTag := names.NewUserTag(EveryoneTagName)
		everyoneAccess, err := accessGetter(usr, everyoneTag, target)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return permission.NoAccess, errors.Trace(err)
		}
		if userAccess == permission.NoAccess && everyoneAccess != permission.NoAccess {
			userAccess = everyoneAccess
		}
		if everyoneAccess.EqualOrGreaterControllerAccessThan(userAccess) {
			userAccess = everyoneAccess
		}
	}
	return userAccess, nil
}

// HasModelAdmin reports whether or not a user has admin access to the
// specified model. A user has model access if they are a controller
// superuser, or if they have been explicitly granted admin access to the
// model.
func HasModelAdmin(
	authorizer facade.Authorizer,
	controllerTag names.ControllerTag,
	modelTag names.ModelTag,
) (bool, error) {
	// superusers have admin for all models.
	err := authorizer.HasPermission(permission.SuperuserAccess, controllerTag)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, err
	}

	if err == nil {
		return true, nil
	}

	err = authorizer.HasPermission(permission.AdminAccess, modelTag)
	return err == nil, err
}
