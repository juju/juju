// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// UserService defines the methods to operate with the database.
type UserService interface {
	GetAllUsers(ctx context.Context) ([]coreuser.User, error)
	GetUserByName(ctx context.Context, name string) (coreuser.User, error)
	AddUser(ctx context.Context, arg service.AddUserArg) (coreuser.UUID, []byte, error)
	EnableUserAuthentication(ctx context.Context, name string) error
	DisableUserAuthentication(ctx context.Context, name string) error
	SetPassword(ctx context.Context, name string, password auth.Password) error
	ResetPassword(ctx context.Context, name string) ([]byte, error)
	RemoveUser(ctx context.Context, name string) error
}

// UserManagerAPI implements the user manager interface and is the concrete
// implementation of the api end point.
type UserManagerAPI struct {
	state       *state.State
	userService UserService
	pool        *state.StatePool
	authorizer  facade.Authorizer
	check       *common.BlockChecker
	apiUserTag  names.UserTag
	apiUser     coreuser.User
	isAdmin     bool
	logger      loggo.Logger
}

// NewAPI creates a new API endpoint for calling user manager functions.
func NewAPI(
	state *state.State,
	userService UserService,
	pool *state.StatePool,
	authorizer facade.Authorizer,
	check *common.BlockChecker,
	apiUserTag names.UserTag,
	apiUser coreuser.User,
	isAdmin bool,
	logger loggo.Logger,
) (*UserManagerAPI, error) {
	return &UserManagerAPI{
		state:       state,
		userService: userService,
		pool:        pool,
		authorizer:  authorizer,
		check:       check,
		apiUserTag:  apiUserTag,
		apiUser:     apiUser,
		isAdmin:     isAdmin,
		logger:      logger,
	}, nil
}

func (api *UserManagerAPI) hasControllerAdminAccess() (bool, error) {
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.state.ControllerTag())
	return err == nil, err
}

// AddUser adds a user with a username, and either a password or
// a randomly generated secret key which will be returned.
func (api *UserManagerAPI) AddUser(ctx context.Context, args params.AddUsers) (params.AddUserResults, error) {
	var result params.AddUserResults

	if _, err := api.hasControllerAdminAccess(); err != nil {
		return result, err
	}

	if err := api.check.ChangeAllowed(ctx); err != nil {
		return result, errors.Trace(err)
	}

	if len(args.Users) == 0 {
		return result, nil
	}

	result.Results = make([]params.AddUserResult, len(args.Users))
	for i, arg := range args.Users {
		result.Results[i] = api.addOneUser(ctx, arg)
	}
	return result, nil
}

func (api *UserManagerAPI) addOneUser(ctx context.Context, arg params.AddUser) params.AddUserResult {
	var activationKey []byte
	var err error

	// TODO(anvial): Legacy block to delete when user domain wire up is complete.
	if arg.Password != "" {
		_, err = api.state.AddUser(arg.Username, arg.DisplayName, arg.Password, api.apiUserTag.Id())
		if err != nil {
			return params.AddUserResult{Error: apiservererrors.ServerError(errors.Annotate(err, "creating user"))}
		}
	} else {
		_, err = api.state.AddUserWithSecretKey(arg.Username, arg.DisplayName, api.apiUserTag.Id())
		if err != nil {
			return params.AddUserResult{Error: apiservererrors.ServerError(errors.Annotate(err, "creating user"))}
		}
	}
	if err != nil {
		return params.AddUserResult{Error: apiservererrors.ServerError(errors.Annotate(err, "creating user"))}
	}
	// End legacy block.

	addUserArg := service.AddUserArg{
		Name:        arg.Username,
		DisplayName: arg.DisplayName,
		CreatorUUID: api.apiUser.UUID,
		Permission:  permission.ControllerForAccess(permission.LoginAccess),
	}
	if arg.Password != "" {
		pass := auth.NewPassword(arg.Password)
		defer pass.Destroy()
		addUserArg.Password = &pass
	}

	if _, activationKey, err = api.userService.AddUser(ctx, addUserArg); err != nil {
		return params.AddUserResult{Error: apiservererrors.ServerError(errors.Annotate(err, "creating user"))}
	}

	return params.AddUserResult{
		Tag:       names.NewLocalUserTag(arg.Username).String(),
		SecretKey: activationKey,
	}
}

// RemoveUser permanently removes a user from the current controller for each
// entity provided. While the user is permanently removed we keep it's
// information around for auditing purposes.
// TODO(redir): Add information about getting deleted user information when we
// add that capability.
func (api *UserManagerAPI) RemoveUser(ctx context.Context, entities params.Entities) (params.ErrorResults, error) {
	var deletions params.ErrorResults

	if err := api.check.ChangeAllowed(ctx); err != nil {
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
		return deletions, apiservererrors.ErrPerm
	}

	// Remove the entities.
	for i, e := range entities.Entities {
		userTag, err := names.ParseUserTag(e.Tag)
		if err != nil {
			deletions.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if controllerOwner.Id() == userTag.Id() {
			deletions.Results[i].Error = apiservererrors.ServerError(
				errors.Errorf("cannot delete controller owner %q", userTag.Name()))
			continue
		}
		// TODO(anvial): Legacy block to delete when user domain wire up is complete.
		err = api.state.RemoveUser(userTag)
		if err != nil {
			deletions.Results[i].Error = apiservererrors.ServerError(
				errors.Annotatef(err, "failed to delete user %q", userTag.Name()))
			continue
		}
		// End legacy block.

		err = api.userService.RemoveUser(ctx, userTag.Name())
		if err != nil {
			deletions.Results[i].Error = apiservererrors.ServerError(
				errors.Annotatef(err, "failed to delete user %q", userTag.Name()))
			continue
		}
		deletions.Results[i].Error = nil

	}
	return deletions, nil
}

// TODO(anvial): Legacy block to delete when user domain wire up is complete.
func (api *UserManagerAPI) getUser(tag string) (*state.User, error) {
	userTag, err := names.ParseUserTag(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	user, err := api.state.User(userTag)
	if err != nil {
		return nil, errors.Wrap(err, apiservererrors.ErrPerm)
	}
	return user, nil
}

// End legacy block.

// EnableUser enables one or more users.  If the user is already enabled,
// the action is considered a success.
func (api *UserManagerAPI) EnableUser(ctx context.Context, users params.Entities) (params.ErrorResults, error) {
	if _, err := api.hasControllerAdminAccess(); err != nil {
		return params.ErrorResults{}, err
	}

	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return api.enableUser(ctx, users, "enable", (*state.User).Enable)
}

// DisableUser disables one or more users.  If the user is already disabled,
// the action is considered a success.
func (api *UserManagerAPI) DisableUser(ctx context.Context, users params.Entities) (params.ErrorResults, error) {
	if _, err := api.hasControllerAdminAccess(); err != nil {
		return params.ErrorResults{}, err
	}

	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return api.enableUser(ctx, users, "disable", (*state.User).Disable)
}

func (api *UserManagerAPI) enableUser(ctx context.Context, args params.Entities, action string, method func(*state.User) error) (params.ErrorResults, error) {
	var result params.ErrorResults

	if len(args.Entities) == 0 {
		return result, nil
	}

	if !api.isAdmin {
		if _, err := api.hasControllerAdminAccess(); err != nil {
			return result, err
		}
	}

	// Create the results list to populate.
	result.Results = make([]params.ErrorResult, len(args.Entities))

	for i, arg := range args.Entities {
		// TODO(anvial): Legacy block to delete when legacyUser domain wire up is complete.
		legacyUser, err := api.getUser(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = method(legacyUser)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Errorf("failed to %s legacyUser: %s", action, err))
			continue
		}
		// End legacy block.

		// Parse User tag
		userTag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// Enable/Disable legacyUser
		if action == "enable" {
			err = api.userService.EnableUserAuthentication(ctx, userTag.Name())
		} else {
			err = api.userService.DisableUserAuthentication(ctx, userTag.Name())
		}
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Errorf("failed to %s legacyUser: %s", action, err))
			continue
		}
	}
	return result, nil
}

// UserInfo returns information on a user.
func (api *UserManagerAPI) UserInfo(ctx context.Context, request params.UserInfoRequest) (params.UserInfoResults, error) {
	var results params.UserInfoResults
	isAdmin, err := api.hasControllerAdminAccess()
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return results, errors.Trace(err)
	}

	var accessForUser = func(userTag names.UserTag, result *params.UserInfoResult) {
		// Lookup the access the specified user has to the controller.
		access, err := common.GetPermission(api.state.UserPermission, userTag, api.state.ControllerTag())
		if err == nil {
			result.Result.Access = string(access)
		} else if err != nil && !errors.Is(err, errors.NotFound) {
			result.Result = nil
			result.Error = apiservererrors.ServerError(err)
		}
	}

	var infoForUser = func(tag names.UserTag, user coreuser.User) params.UserInfoResult {
		result := params.UserInfoResult{
			Result: &params.UserInfo{
				Username:       user.Name,
				DisplayName:    user.DisplayName,
				CreatedBy:      user.CreatorName,
				DateCreated:    user.CreatedAt,
				LastConnection: &user.LastLogin,
				Disabled:       user.Disabled,
			},
		}
		if user.Disabled {
			// disabled users have no access to the controller.
			result.Result.Access = string(permission.NoAccess)
		} else {
			accessForUser(tag, &result)
		}
		return result
	}

	argCount := len(request.Entities)
	if argCount == 0 {

		if isAdmin {
			// Get all users if isAdmin
			users, err := api.userService.GetAllUsers(ctx)
			if err != nil {
				return results, errors.Trace(err)
			}
			for _, user := range users {
				userTag := names.NewLocalUserTag(user.Name)
				results.Results = append(results.Results, infoForUser(userTag, user))
			}
			return results, nil
		}

		// If not admin, get users filtered by the auth tag. We just need to
		// check if the auth tag is a user tag, and if so, get the users filtered
		// by the user name.
		tag := api.authorizer.GetAuthTag()
		if _, ok := tag.(names.UserTag); !ok {
			return results, apiservererrors.ErrPerm
		}

		// Get users filtered by the apiUser name as a creator
		user, err := api.userService.GetUserByName(ctx, tag.Id())
		if err != nil {
			return results, errors.Trace(err)
		}

		userTag := names.NewLocalUserTag(user.Name)
		results.Results = append(results.Results, infoForUser(userTag, user))

		return results, nil
	}

	// Create the results list to populate.
	for _, arg := range request.Entities {
		userTag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			results.Results = append(results.Results, params.UserInfoResult{Error: apiservererrors.ServerError(err)})
			continue
		}
		if !isAdmin && !api.authorizer.AuthOwner(userTag) {
			results.Results = append(results.Results, params.UserInfoResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)})
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

		// Get User
		user, err := api.getLocalUserByTag(ctx, arg.Tag)
		if err != nil {
			results.Results = append(results.Results, params.UserInfoResult{Error: apiservererrors.ServerError(err)})
			continue
		}
		results.Results = append(results.Results, infoForUser(userTag, user))
	}

	return results, nil
}

func (api *UserManagerAPI) checkCanRead(modelTag names.Tag) error {
	return api.authorizer.HasPermission(permission.ReadAccess, modelTag)
}

// ModelUserInfo returns information on all users in the model.
func (api *UserManagerAPI) ModelUserInfo(ctx context.Context, args params.Entities) (params.ModelUserInfoResults, error) {
	var result params.ModelUserInfoResults
	for _, entity := range args.Entities {
		modelTag, err := names.ParseModelTag(entity.Tag)
		if err != nil {
			return result, errors.Trace(err)
		}
		infos, err := api.modelUserInfo(modelTag)
		if err != nil {
			return result, errors.Trace(err)
		}
		result.Results = append(result.Results, infos...)
	}
	return result, nil
}

func (api *UserManagerAPI) modelUserInfo(modelTag names.ModelTag) ([]params.ModelUserInfoResult, error) {
	var results []params.ModelUserInfoResult
	model, closer, err := api.pool.GetModel(modelTag.Id())
	if err != nil {
		return results, errors.Trace(err)
	}
	defer closer.Release()
	if err := api.checkCanRead(model.ModelTag()); err != nil {
		return results, err
	}

	users, err := model.Users()
	if err != nil {
		return results, errors.Trace(err)
	}

	for _, user := range users {
		var result params.ModelUserInfoResult
		userInfo, err := common.ModelUserInfo(user, model)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			userInfo.ModelTag = modelTag.String()
			result.Result = &userInfo
		}
		results = append(results, result)
	}
	return results, nil
}

// SetPassword changes the stored password for the specified users.
func (api *UserManagerAPI) SetPassword(ctx context.Context, args params.EntityPasswords) (params.ErrorResults, error) {
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	var result params.ErrorResults

	if len(args.Changes) == 0 {
		return result, nil
	}

	// Create the results list to populate.
	result.Results = make([]params.ErrorResult, len(args.Changes))
	for i, arg := range args.Changes {
		if err := api.setPassword(ctx, arg); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

func (api *UserManagerAPI) setPassword(ctx context.Context, arg params.EntityPassword) error {
	// TODO(anvial): Legacy block to delete when legacyUser domain wire up is complete.
	legacyUser, err := api.getUser(arg.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	// End legacy block.

	if !api.isAdmin {
		userTag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		if _, err := api.hasControllerAdminAccess(); err != nil && api.apiUserTag != userTag {
			return err
		}
	}

	if strings.TrimSpace(arg.Password) == "" {
		return errors.New("cannot use an empty password")
	}

	// TODO(anvial): Legacy block to delete when legacyUser domain wire up is complete.
	if err := legacyUser.SetPassword(arg.Password); err != nil {
		return errors.Annotate(err, "failed to set password")
	}
	// End legacy block.

	userTag, err := names.ParseUserTag(arg.Tag)
	if err != nil {
		return errors.Trace(err)
	}

	pass := auth.NewPassword(arg.Password)
	defer pass.Destroy()

	if err := api.userService.SetPassword(ctx, userTag.Name(), pass); err != nil {
		return errors.Annotate(err, "failed to set password")
	}

	return nil
}

// ResetPassword resets password for supplied users by
// invalidating current passwords (if any) and generating
// new random secret keys which will be returned.
// Users cannot reset their own password.
func (api *UserManagerAPI) ResetPassword(ctx context.Context, args params.Entities) (params.AddUserResults, error) {
	var result params.AddUserResults

	if err := api.check.ChangeAllowed(ctx); err != nil {
		return result, errors.Trace(err)
	}

	if len(args.Entities) == 0 {
		return result, nil
	}

	isSuperUser, err := api.hasControllerAdminAccess()
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return result, errors.Trace(err)
	}

	result.Results = make([]params.AddUserResult, len(args.Entities))
	for i, arg := range args.Entities {
		result.Results[i] = params.AddUserResult{Tag: arg.Tag}

		// TODO(anvial): Legacy block to delete when legacyUser domain wire up is complete.
		legacyUser, err := api.getUser(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if isSuperUser && api.apiUserTag != legacyUser.Tag() {
			_, err := legacyUser.ResetPassword()
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
		}
		// End legacy block.

		// Parse User tag
		userTag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// Reset password
		if isSuperUser && api.apiUserTag != userTag {
			key, err := api.userService.ResetPassword(ctx, userTag.Name())
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			result.Results[i].SecretKey = key
		} else {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
		}
	}
	return result, nil
}

// getLocalUserByTag returns the local user with the given tag. It returns an
// error if the tag is not a valid local user tag.
func (api *UserManagerAPI) getLocalUserByTag(ctx context.Context, tag string) (coreuser.User, error) {
	userTag, err := names.ParseUserTag(tag)
	if err != nil {
		return coreuser.User{}, errors.Trace(err)
	}
	if !userTag.IsLocal() {
		return coreuser.User{}, errors.Errorf("%q is not a local user", userTag)
	}
	return api.userService.GetUserByName(ctx, userTag.Name())
}
