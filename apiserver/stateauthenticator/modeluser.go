// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
)

// modelUserEntityFinder implements state.EntityFinder by returning
// an Entity value for model users, ensuring that the user exists in
// the state's current model, while also supporting external users.
type modelUserEntityFinder struct {
	st          *state.State
	userService UserService
}

// FindEntity implements state.EntityFinder.
func (f modelUserEntityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	utag, ok := tag.(names.UserTag)
	if !ok {
		return f.st.FindEntity(tag)
	}

	usr, err := f.userService.GetUserByName(context.Background(), utag.Name())
	if err != nil {
		return nil, errors.Trace(err)
	}

	model, err := f.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelAccess, err := f.st.UserAccess(usr, utag, model.ModelTag())
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}

	// No model user found, so see if the user has been granted
	// access to the controller.
	if permission.IsEmptyUserAccess(modelAccess) {
		controllerAccess, err := state.ControllerAccess(usr, f.st, utag)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(err)
		}
		// TODO(perrito666) remove the following section about everyone group
		// when groups are implemented, this accounts only for the lack of a local
		// ControllerUser when logging in from an external user that has not been granted
		// permissions on the controller but there are permissions for the special
		// everyone group.
		if permission.IsEmptyUserAccess(controllerAccess) && !utag.IsLocal() {
			everyoneTag := names.NewUserTag(common.EveryoneTagName)
			controllerAccess, err = f.st.UserAccess(usr, everyoneTag, f.st.ControllerTag())
			if err != nil && !errors.Is(err, errors.NotFound) {
				return nil, errors.Annotatef(err, "obtaining ControllerUser for everyone group")
			}
		}
		if permission.IsEmptyUserAccess(controllerAccess) {
			return nil, errors.NotFoundf("model or controller user")
		}
	}

	u := &modelUserEntity{
		st:          f.st,
		modelAccess: modelAccess,
		tag:         utag,
		userService: f.userService,
	}
	if utag.IsLocal() {
		user, err := f.userService.GetUserByName(context.Background(), utag.Name())
		if err != nil {
			return nil, errors.Trace(err)
		}
		u.user = user
	}
	return u, nil
}

// modelUserEntity encapsulates a model user
// and, if the user is local, the local state user
// as well. This enables us to implement FindEntity
// in such a way that the authentication mechanisms
// can work without knowing these details.
type modelUserEntity struct {
	st *state.State

	modelAccess permission.UserAccess
	tag         names.Tag

	user coreuser.User

	userService UserService
}

// Refresh implements state.Authenticator.Refresh.
func (u *modelUserEntity) Refresh() error {
	var err error
	u.user, err = u.userService.GetUserByName(context.Background(), u.user.Name)
	return errors.Trace(err)
}

// SetPassword implements state.Authenticator.SetPassword
// by setting the password on the local user.
func (u *modelUserEntity) SetPassword(pass string) error {
	return u.userService.SetPassword(context.Background(), u.user.UUID, auth.NewPassword(pass))
}

// PasswordValid implements state.Authenticator.PasswordValid.
func (u *modelUserEntity) PasswordValid(pass string) bool {
	// TODO(anvial): Implement password validation for external users.
	return true
}

// Tag implements state.Entity.Tag.
func (u *modelUserEntity) Tag() names.Tag {
	return u.tag
}

// LastLogin implements loginEntity.LastLogin.
func (u *modelUserEntity) LastLogin() (time.Time, error) {
	// The last connection for the model takes precedence over
	// the local user last login time.
	var err error
	var t time.Time

	model, err := u.st.Model()
	if err != nil {
		return t, errors.Trace(err)
	}

	if !permission.IsEmptyUserAccess(u.modelAccess) {
		t, err = model.LastModelConnection(u.modelAccess.UserTag)
	} else {
		err = stateerrors.NewNeverConnectedError("controller user")
	}
	if stateerrors.IsNeverConnectedError(err) || permission.IsEmptyUserAccess(u.modelAccess) {
		return u.user.LastLogin, nil
	}
	return t, errors.Trace(err)
}

// UpdateLastLogin implements loginEntity.UpdateLastLogin.
func (u *modelUserEntity) UpdateLastLogin() error {
	updateLastLogin := func() error {
		if err := u.userService.UpdateLastLogin(context.Background(), u.user.UUID); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	if !permission.IsEmptyUserAccess(u.modelAccess) {
		if u.modelAccess.Object.Kind() != names.ModelTagKind {
			return errors.NotValidf("%s as model user", u.modelAccess.Object.Kind())
		}

		model, err := u.st.Model()
		if err != nil {
			return errors.Trace(err)
		}

		if err := model.UpdateLastModelConnection(u.modelAccess.UserTag); err != nil {
			// Attempt to update the users last login data, if the update
			// fails, then just report it as a log message and return the
			// original error message.
			if err := updateLastLogin(); err != nil {
				logger.Warningf("Unable to update last login with %s", err.Error())
			}
			return errors.Trace(err)
		}
	}

	return updateLastLogin()
}
