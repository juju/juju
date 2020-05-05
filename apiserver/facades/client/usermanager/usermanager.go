// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.usermanager")

// UserManagerAPI implements the user manager interface and is the concrete
// implementation of the api end point.
type UserManagerAPI struct {
	state      *state.State
	authorizer facade.Authorizer
	check      *common.BlockChecker
	apiUser    names.UserTag
	isAdmin    bool
}

// NewUserManagerAPI provides the signature required for facade registration.
func NewUserManagerAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*UserManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)
	// Pretty much all of the user manager methods have special casing for admin
	// users, so look once when we start and remember if the user is an admin.
	isAdmin, err := authorizer.HasPermission(permission.SuperuserAccess, st.ControllerTag())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &UserManagerAPI{
		state:      st,
		authorizer: authorizer,
		check:      common.NewBlockChecker(st),
		apiUser:    apiUser,
		isAdmin:    isAdmin,
	}, nil
}

func (api *UserManagerAPI) hasControllerAdminAccess() (bool, error) {
	isAdmin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.state.ControllerTag())
	if errors.IsNotFound(err) {
		return false, nil
	}
	return isAdmin, err
}

// AddUser adds a user with a username, and either a password or
// a randomly generated secret key which will be returned.
func (api *UserManagerAPI) AddUser(args params.AddUsers) (params.AddUserResults, error) {
	var result params.AddUserResults

	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}

	if len(args.Users) == 0 {
		return result, nil
	}

	// Create the results list to populate.
	result.Results = make([]params.AddUserResult, len(args.Users))

	isSuperUser, err := api.hasControllerAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !isSuperUser {
		return result, common.ErrPerm
	}

	for i, arg := range args.Users {
		var user *state.User
		var err error
		if arg.Password != "" {
			user, err = api.state.AddUser(arg.Username, arg.DisplayName, arg.Password, api.apiUser.Id())
		} else {
			user, err = api.state.AddUserWithSecretKey(arg.Username, arg.DisplayName, api.apiUser.Id())
		}
		if err != nil {
			err = errors.Annotate(err, "failed to create user")
			result.Results[i].Error = common.ServerError(err)
			continue
		} else {
			result.Results[i] = params.AddUserResult{
				Tag:       user.Tag().String(),
				SecretKey: user.SecretKey(),
			}
		}

	}
	return result, nil
}

// RemoveUser permanently removes a user from the current controller for each
// entity provided. While the user is permanently removed we keep it's
// information around for auditing purposes.
// TODO(redir): Add information about getting deleted user information when we
// add that capability.
func (api *UserManagerAPI) RemoveUser(entities params.Entities) (params.ErrorResults, error) {
	var deletions params.ErrorResults

	if err := api.check.ChangeAllowed(); err != nil {
		return deletions, errors.Trace(err)
	}

	controllerOwner, err := api.state.ControllerOwner()
	if err != nil {
		return deletions, errors.Trace(err)
	}

	// Create the results list to populate.
	deletions.Results = make([]params.ErrorResult, len(entities.Entities))

	isSuperUser, err := api.hasControllerAdminAccess()
	if err != nil {
		return deletions, errors.Trace(err)
	}
	if !api.isAdmin && !isSuperUser {
		return deletions, common.ErrPerm
	}

	// Remove the entities.
	for i, e := range entities.Entities {
		user, err := names.ParseUserTag(e.Tag)
		if err != nil {
			deletions.Results[i].Error = common.ServerError(err)
			continue
		}

		if controllerOwner.Id() == user.Id() {
			deletions.Results[i].Error = common.ServerError(
				errors.Errorf("cannot delete controller owner %q", user.Name()))
			continue
		}
		err = api.state.RemoveUser(user)
		if err != nil {
			if errors.IsUserNotFound(err) {
				deletions.Results[i].Error = common.ServerError(err)
			} else {
				deletions.Results[i].Error = common.ServerError(
					errors.Annotatef(err, "failed to delete user %q", user.Name()))
			}
			continue
		}
		deletions.Results[i].Error = nil
	}
	return deletions, nil
}

func (api *UserManagerAPI) getUser(tag string) (*state.User, error) {
	userTag, err := names.ParseUserTag(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	user, err := api.state.User(userTag)
	if err != nil {
		return nil, errors.Wrap(err, common.ErrPerm)
	}
	return user, nil
}

// EnableUser enables one or more users.  If the user is already enabled,
// the action is considered a success.
func (api *UserManagerAPI) EnableUser(users params.Entities) (params.ErrorResults, error) {
	isSuperUser, err := api.hasControllerAdminAccess()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if !isSuperUser {
		return params.ErrorResults{}, common.ErrPerm
	}

	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return api.enableUserImpl(users, "enable", (*state.User).Enable)
}

// DisableUser disables one or more users.  If the user is already disabled,
// the action is considered a success.
func (api *UserManagerAPI) DisableUser(users params.Entities) (params.ErrorResults, error) {
	isSuperUser, err := api.hasControllerAdminAccess()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if !isSuperUser {
		return params.ErrorResults{}, common.ErrPerm
	}

	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return api.enableUserImpl(users, "disable", (*state.User).Disable)
}

func (api *UserManagerAPI) enableUserImpl(args params.Entities, action string, method func(*state.User) error) (params.ErrorResults, error) {
	var result params.ErrorResults

	if len(args.Entities) == 0 {
		return result, nil
	}

	isSuperUser, err := api.hasControllerAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}

	if !api.isAdmin && isSuperUser {
		return result, common.ErrPerm
	}

	// Create the results list to populate.
	result.Results = make([]params.ErrorResult, len(args.Entities))

	for i, arg := range args.Entities {
		user, err := api.getUser(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = method(user)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Errorf("failed to %s user: %s", action, err))
		}
	}
	return result, nil
}

// UserInfo returns information on a user.
func (api *UserManagerAPI) UserInfo(request params.UserInfoRequest) (params.UserInfoResults, error) {
	var results params.UserInfoResults
	isAdmin, err := api.hasControllerAdminAccess()
	if err != nil {
		return results, errors.Trace(err)
	}

	var accessForUser = func(userTag names.UserTag, result *params.UserInfoResult) {
		// Lookup the access the specified user has to the controller.
		access, err := common.GetPermission(api.state.UserPermission, userTag, api.state.ControllerTag())
		if err == nil {
			result.Result.Access = string(access)
		} else if err != nil && !errors.IsNotFound(err) {
			result.Result = nil
			result.Error = common.ServerError(err)
		}
	}

	var infoForUser = func(user *state.User) params.UserInfoResult {
		var lastLogin *time.Time
		userLastLogin, err := user.LastLogin()
		if err != nil {
			if !state.IsNeverLoggedInError(err) {
				logger.Debugf("error getting last login: %v", err)
			}
		} else {
			lastLogin = &userLastLogin
		}
		result := params.UserInfoResult{
			Result: &params.UserInfo{
				Username:       user.Name(),
				DisplayName:    user.DisplayName(),
				CreatedBy:      user.CreatedBy(),
				DateCreated:    user.DateCreated(),
				LastConnection: lastLogin,
				Disabled:       user.IsDisabled(),
			},
		}
		if user.IsDisabled() {
			// disabled users have no access to the controller.
			result.Result.Access = string(permission.NoAccess)
		} else {
			accessForUser(user.UserTag(), &result)
		}
		return result
	}

	argCount := len(request.Entities)
	if argCount == 0 {
		users, err := api.state.AllUsers(request.IncludeDisabled)
		if err != nil {
			return results, errors.Trace(err)
		}
		for _, user := range users {
			if !isAdmin && !api.authorizer.AuthOwner(user.Tag()) {
				continue
			}
			results.Results = append(results.Results, infoForUser(user))
		}
		return results, nil
	}

	// Create the results list to populate.
	for _, arg := range request.Entities {
		userTag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			results.Results = append(results.Results, params.UserInfoResult{Error: common.ServerError(err)})
			continue
		}
		if !isAdmin && !api.authorizer.AuthOwner(userTag) {
			results.Results = append(results.Results, params.UserInfoResult{Error: common.ServerError(common.ErrPerm)})
			continue
		}
		if !userTag.IsLocal() {
			// TODO(wallyworld) record login information about external users.
			result := params.UserInfoResult{
				Result: &params.UserInfo{
					Username: userTag.Id(),
				},
			}
			accessForUser(userTag, &result)
			results.Results = append(results.Results, result)
			continue
		}
		user, err := api.getUser(arg.Tag)
		if err != nil {
			results.Results = append(results.Results, params.UserInfoResult{Error: common.ServerError(err)})
			continue
		}
		results.Results = append(results.Results, infoForUser(user))
	}

	return results, nil
}

// SetPassword changes the stored password for the specified users.
func (api *UserManagerAPI) SetPassword(args params.EntityPasswords) (params.ErrorResults, error) {
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	var result params.ErrorResults

	if len(args.Changes) == 0 {
		return result, nil
	}

	// Create the results list to populate.
	result.Results = make([]params.ErrorResult, len(args.Changes))
	for i, arg := range args.Changes {
		if err := api.setPassword(arg); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

func (api *UserManagerAPI) setPassword(arg params.EntityPassword) error {
	user, err := api.getUser(arg.Tag)
	if err != nil {
		return errors.Trace(err)
	}

	isSuperUser, err := api.hasControllerAdminAccess()
	if err != nil {
		return errors.Trace(err)
	}

	if api.apiUser != user.UserTag() && !api.isAdmin && !isSuperUser {
		return errors.Trace(common.ErrPerm)
	}
	if arg.Password == "" {
		return errors.New("cannot use an empty password")
	}
	if err := user.SetPassword(arg.Password); err != nil {
		return errors.Annotate(err, "failed to set password")
	}
	return nil
}

// ResetPassword resets password for supplied users by
// invalidating current passwords (if any) and generating
// new random secret keys which will be returned.
// Users cannot reset their own password.
func (api *UserManagerAPI) ResetPassword(args params.Entities) (params.AddUserResults, error) {
	var result params.AddUserResults

	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}

	if len(args.Entities) == 0 {
		return result, nil
	}

	isSuperUser, err := api.hasControllerAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Results = make([]params.AddUserResult, len(args.Entities))
	for i, arg := range args.Entities {
		result.Results[i] = params.AddUserResult{Tag: arg.Tag}
		user, err := api.getUser(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if isSuperUser && api.apiUser != user.Tag() {
			key, err := user.ResetPassword()
			if err != nil {
				result.Results[i].Error = common.ServerError(err)
				continue
			}
			result.Results[i].SecretKey = key
		} else {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
		}
	}
	return result, nil
}
