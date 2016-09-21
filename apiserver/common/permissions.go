// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

// EveryoneTagName represents a special group that encompasses
// all external users.
const EveryoneTagName = "everyone@external"

// UserAccess returns the access the user has on the model state
// and the host controller.
func UserAccess(st *state.State, utag names.UserTag) (modelUser, controllerUser permission.UserAccess, err error) {
	var none permission.UserAccess
	modelUser, err = st.UserAccess(utag, st.ModelTag())
	if err != nil && !errors.IsNotFound(err) {
		return none, none, errors.Trace(err)
	}

	controllerUser, err = state.ControllerAccess(st, utag)
	if err != nil && !errors.IsNotFound(err) {
		return none, none, errors.Trace(err)
	}

	// TODO(perrito666) remove the following section about everyone group
	// when groups are implemented, this accounts only for the lack of a local
	// ControllerUser when logging in from an external user that has not been granted
	// permissions on the controller but there are permissions for the special
	// everyone group.
	if !utag.IsLocal() {
		controllerUser, err = maybeUseGroupPermission(st.UserAccess, controllerUser, st.ControllerTag(), utag)
		if err != nil {
			return none, none, errors.Annotatef(err, "obtaining ControllerUser for everyone group")
		}
	}

	if permission.IsEmptyUserAccess(modelUser) &&
		permission.IsEmptyUserAccess(controllerUser) {
		return none, none, errors.NotFoundf("model or controller user")
	}
	return modelUser, controllerUser, nil
}

// HasPermission returns true if the specified user has the specified
// permission on target.
func HasPermission(userGetter userAccessFunc, utag names.Tag,
	requestedPermission permission.Access, target names.Tag) (bool, error) {

	validForKind := false
	switch requestedPermission {
	case permission.LoginAccess, permission.AddModelAccess, permission.SuperuserAccess:
		validForKind = target.Kind() == names.ControllerTagKind
	case permission.ReadAccess, permission.WriteAccess, permission.AdminAccess:
		validForKind = target.Kind() == names.ModelTagKind
	}

	if !validForKind {
		return false, nil
	}

	userTag, ok := utag.(names.UserTag)
	if !ok {
		// lets not reveal more than is strictly necessary
		return false, nil
	}

	user, err := userGetter(userTag, target)
	if err != nil && !errors.IsNotFound(err) {
		return false, errors.Annotatef(err, "while obtaining %s user", target.Kind())
	}
	// there is a special case for external users, a group called everyone@external
	if target.Kind() == names.ControllerTagKind && !userTag.IsLocal() {
		controllerTag, ok := target.(names.ControllerTag)
		if !ok {
			return false, errors.NotValidf("controller tag")
		}

		// TODO(perrito666) remove the following section about everyone group
		// when groups are implemented, this accounts only for the lack of a local
		// ControllerUser when logging in from an external user that has not been granted
		// permissions on the controller but there are permissions for the special
		// everyone group.
		user, err = maybeUseGroupPermission(userGetter, user, controllerTag, userTag)
		if err != nil {
			return false, errors.Trace(err)
		}
		if permission.IsEmptyUserAccess(user) {
			return false, nil
		}
	}
	// returning this kind of information would be too much information to reveal too.
	if errors.IsNotFound(err) {
		return false, nil
	}
	modelPermission := user.Access.EqualOrGreaterModelAccessThan(requestedPermission) && target.Kind() == names.ModelTagKind
	controllerPermission := user.Access.EqualOrGreaterControllerAccessThan(requestedPermission) && target.Kind() == names.ControllerTagKind
	if !controllerPermission && !modelPermission {
		return false, nil
	}
	return true, nil
}

// maybeUseGroupPermission returns a permission.UserAccess updated
// with the group permissions that apply to it if higher than
// current.
// If the passed UserAccess is empty (controller user lacks permissions)
// but the group is not, a stand-in will be created to hold the group
// permissions.
func maybeUseGroupPermission(
	userGetter userAccessFunc,
	externalUser permission.UserAccess,
	controllerTag names.ControllerTag,
	userTag names.UserTag,
) (permission.UserAccess, error) {

	everyoneTag := names.NewUserTag(EveryoneTagName)
	everyone, err := userGetter(everyoneTag, controllerTag)
	if errors.IsNotFound(err) {
		return externalUser, nil
	}
	if err != nil {
		return permission.UserAccess{}, errors.Trace(err)
	}
	if permission.IsEmptyUserAccess(externalUser) &&
		!permission.IsEmptyUserAccess(everyone) {
		externalUser = newControllerUserFromGroup(everyone, userTag)
	}

	if everyone.Access.EqualOrGreaterControllerAccessThan(externalUser.Access) {
		externalUser.Access = everyone.Access
	}
	return externalUser, nil
}

type userAccessFunc func(names.UserTag, names.Tag) (permission.UserAccess, error)

// newControllerUserFromGroup returns a permission.UserAccess that serves
// as a stand-in for a user that has group access but no explicit user
// access.
func newControllerUserFromGroup(everyoneAccess permission.UserAccess,
	userTag names.UserTag) permission.UserAccess {
	everyoneAccess.UserTag = userTag
	everyoneAccess.UserID = strings.ToLower(userTag.Canonical())
	everyoneAccess.UserName = userTag.Canonical()
	return everyoneAccess
}
