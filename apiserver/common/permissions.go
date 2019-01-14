// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/permission"
)

// EveryoneTagName represents a special group that encompasses
// all external users.
const EveryoneTagName = "everyone@external"

type userAccessFunc func(names.UserTag, names.Tag) (permission.Access, error)

// HasPermission returns true if the specified user has the specified
// permission on target.
func HasPermission(
	accessGetter userAccessFunc, utag names.Tag,
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

	userAccess, err := GetPermission(accessGetter, userTag, target)
	if err != nil && !errors.IsNotFound(err) {
		return false, errors.Annotatef(err, "while obtaining %s user", target.Kind())
	}
	// returning this kind of information would be too much information to reveal too.
	if errors.IsNotFound(err) || userAccess == permission.NoAccess {
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
func GetPermission(accessGetter userAccessFunc, userTag names.UserTag, target names.Tag) (permission.Access, error) {
	userAccess, err := accessGetter(userTag, target)
	if err != nil && !errors.IsNotFound(err) {
		return permission.NoAccess, errors.Annotatef(err, "while obtaining %s user", target.Kind())
	}
	// there is a special case for external users, a group called everyone@external
	if !userTag.IsLocal() {
		// TODO(perrito666) remove the following section about everyone group
		// when groups are implemented.
		everyoneTag := names.NewUserTag(EveryoneTagName)
		everyoneAccess, err := accessGetter(everyoneTag, target)
		if err != nil && !errors.IsNotFound(err) {
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
	user names.UserTag,
	controllerTag names.ControllerTag,
	model Model,
) (bool, error) {
	// superusers have admin for all models.
	if isSuperUser, err := authorizer.HasPermission(permission.SuperuserAccess, controllerTag); err != nil || isSuperUser {
		return isSuperUser, err
	}
	return authorizer.HasPermission(permission.AdminAccess, model.ModelTag())
}
