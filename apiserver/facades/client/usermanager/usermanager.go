// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"context"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/auth"
	interrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// AccessService defines the methods to operate with the database.
type AccessService interface {
	GetAllUsers(ctx context.Context, includeDisabled bool) ([]coreuser.User, error)
	GetUserByName(ctx context.Context, name coreuser.Name) (coreuser.User, error)
	GetUser(ctx context.Context, uuid coreuser.UUID) (coreuser.User, error)
	AddUser(ctx context.Context, arg service.AddUserArg) (coreuser.UUID, []byte, error)
	EnableUserAuthentication(ctx context.Context, name coreuser.Name) error
	DisableUserAuthentication(ctx context.Context, name coreuser.Name) error
	SetPassword(ctx context.Context, name coreuser.Name, password auth.Password) error
	ResetPassword(ctx context.Context, name coreuser.Name) ([]byte, error)
	RemoveUser(ctx context.Context, name coreuser.Name) error

	// ReadUserAccessLevelForTarget returns the access level that the
	// input user has been on the input target entity.
	// If the access level of a user cannot be found then
	// accesserrors.AccessNotFound is returned.
	ReadUserAccessLevelForTarget(ctx context.Context, subject coreuser.Name, target permission.ID) (permission.Access, error)
}

// ModelService defines an interface for interacting with the model service.
type ModelService interface {
	// ControllerModel returns the model used for housing the Juju controller.
	// Should no model exist for the controller an error of [modelerrors.NotFound]
	// will be returned.
	ControllerModel(ctx context.Context) (coremodel.Model, error)

	// GetModelUsers will retrieve basic information about users with
	// permissions on the given model UUID.
	// If the model cannot be found it will return
	// [github.com/juju/juju/domain/model/errors.NotFound].
	GetModelUsers(ctx context.Context, modelUUID coremodel.UUID) ([]coremodel.ModelUserInfo, error)

	// GetModelUser will retrieve basic information about the specified model
	// user.
	// If the model cannot be found it will return
	// [github.com/juju/juju/domain/model/errors.NotFound].
	// If the user cannot be found it will return
	// [github.com/juju/juju/domain/model/errors.UserNotFoundOnModel].
	GetModelUser(ctx context.Context, modelUUID coremodel.UUID, name coreuser.Name) (coremodel.ModelUserInfo, error)
}

// UserManagerAPI implements the user manager interface and is the concrete
// implementation of the api end point.
type UserManagerAPI struct {
	accessService  AccessService
	modelService   ModelService
	authorizer     facade.Authorizer
	check          *common.BlockChecker
	apiUserTag     names.UserTag
	apiUser        coreuser.User
	isAdmin        bool
	logger         corelogger.Logger
	controllerUUID string
}

// NewAPI creates a new API endpoint for calling user manager functions.
func NewAPI(
	accessService AccessService,
	modelService ModelService,
	authorizer facade.Authorizer,
	check *common.BlockChecker,
	apiUserTag names.UserTag,
	apiUser coreuser.User,
	isAdmin bool,
	logger corelogger.Logger,
	controllerUUID string,
) (*UserManagerAPI, error) {
	return &UserManagerAPI{
		accessService:  accessService,
		modelService:   modelService,
		authorizer:     authorizer,
		check:          check,
		apiUserTag:     apiUserTag,
		apiUser:        apiUser,
		isAdmin:        isAdmin,
		logger:         logger,
		controllerUUID: controllerUUID,
	}, nil
}

func (api *UserManagerAPI) hasControllerAdminAccess(ctx context.Context) (bool, error) {
	err := api.authorizer.HasPermission(ctx, permission.SuperuserAccess, names.NewControllerTag(api.controllerUUID))
	return err == nil, err
}

// AddUser adds a user with a username, and either a password or
// a randomly generated secret key which will be returned.
func (api *UserManagerAPI) AddUser(ctx context.Context, args params.AddUsers) (params.AddUserResults, error) {
	var result params.AddUserResults

	if _, err := api.hasControllerAdminAccess(ctx); err != nil {
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

	name, err := coreuser.NewName(arg.Username)
	if err != nil {
		return params.AddUserResult{Error: apiservererrors.ServerError(errors.Annotate(err, "creating user"))}
	}

	addUserArg := service.AddUserArg{
		Name:        name,
		DisplayName: arg.DisplayName,
		CreatorUUID: api.apiUser.UUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        api.controllerUUID,
			},
		},
	}
	if arg.Password != "" {
		pass := auth.NewPassword(arg.Password)
		defer pass.Destroy()
		addUserArg.Password = &pass
	}

	if _, activationKey, err = api.accessService.AddUser(ctx, addUserArg); err != nil {
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

	// Create the results list to populate.
	deletions.Results = make([]params.ErrorResult, len(entities.Entities))

	isSuperUser, err := api.hasControllerAdminAccess(ctx)
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

		// TODO - check if user is the last admin of any models
		//  do not allow last admin to be deleted.
		if environs.AdminUser == userTag.Id() {
			deletions.Results[i].Error = apiservererrors.ServerError(
				errors.Errorf("cannot delete controller owner %q", userTag.Name()))
			continue
		}

		err = api.accessService.RemoveUser(ctx, coreuser.NameFromTag(userTag))
		if err != nil {
			deletions.Results[i].Error = apiservererrors.ServerError(
				errors.Annotatef(err, "failed to delete user %q", userTag.Name()))
			continue
		}
		deletions.Results[i].Error = nil

	}
	return deletions, nil
}

// EnableUser enables one or more users.  If the user is already enabled,
// the action is considered a success.
func (api *UserManagerAPI) EnableUser(ctx context.Context, users params.Entities) (params.ErrorResults, error) {
	if _, err := api.hasControllerAdminAccess(ctx); err != nil {
		return params.ErrorResults{}, err
	}

	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return api.enableUser(ctx, users, "enable")
}

// DisableUser disables one or more users.  If the user is already disabled,
// the action is considered a success.
func (api *UserManagerAPI) DisableUser(ctx context.Context, users params.Entities) (params.ErrorResults, error) {
	if _, err := api.hasControllerAdminAccess(ctx); err != nil {
		return params.ErrorResults{}, err
	}

	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return api.enableUser(ctx, users, "disable")
}

func (api *UserManagerAPI) enableUser(ctx context.Context, args params.Entities, action string) (params.ErrorResults, error) {
	var result params.ErrorResults

	if len(args.Entities) == 0 {
		return result, nil
	}

	if !api.isAdmin {
		if _, err := api.hasControllerAdminAccess(ctx); err != nil {
			return result, err
		}
	}

	result.Results = make([]params.ErrorResult, len(args.Entities))
	for i, arg := range args.Entities {
		userTag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if action == "enable" {
			err = api.accessService.EnableUserAuthentication(ctx, coreuser.NameFromTag(userTag))
		} else {
			err = api.accessService.DisableUserAuthentication(ctx, coreuser.NameFromTag(userTag))
		}
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Errorf("failed to %s user: %s", action, err))
			continue
		}
	}
	return result, nil
}

// UserInfo returns information on a user.
func (api *UserManagerAPI) UserInfo(ctx context.Context, request params.UserInfoRequest) (params.UserInfoResults, error) {
	var results params.UserInfoResults
	isAdmin, err := api.hasControllerAdminAccess(ctx)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return results, errors.Trace(err)
	}

	argCount := len(request.Entities)
	if argCount == 0 {
		if isAdmin {
			// Get all users if isAdmin
			users, err := api.accessService.GetAllUsers(ctx, request.IncludeDisabled)
			if err != nil {
				return results, errors.Trace(err)
			}
			for _, user := range users {
				userTag := names.NewUserTag(user.Name.Name())
				results.Results = append(results.Results, api.infoForUser(ctx, userTag, user))
			}
			return results, nil
		}

		// If not admin, get users filtered by the auth tag. We just need to
		// check if the auth tag is a user tag, and if so, get the users filtered
		// by the user name.
		tag := api.authorizer.GetAuthTag()
		userTag, ok := tag.(names.UserTag)
		if !ok {
			return results, apiservererrors.ErrPerm
		}

		// Get users filtered by the apiUser name as a creator
		user, err := api.accessService.GetUserByName(ctx, coreuser.NameFromTag(userTag))
		if err != nil {
			return results, errors.Trace(err)
		}

		results.Results = append(results.Results, api.infoForUser(ctx, userTag, user))

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

		// Get User
		user, err := api.accessService.GetUserByName(ctx, coreuser.NameFromTag(userTag))
		if errors.Is(err, accesserrors.UserNotFound) {
			err = interrors.Errorf("user %q not found", userTag.Name()).Add(coreerrors.UserNotFound)
		}

		if err != nil {
			results.Results = append(results.Results, params.UserInfoResult{Error: apiservererrors.ServerError(err)})
			continue
		}
		results.Results = append(results.Results, api.infoForUser(ctx, userTag, user))
	}

	return results, nil
}

// infoForUser generates a UserInfoResult from a coreuser.User, it fills in
// information needed but not contained in the core user from the access
// service.
func (api *UserManagerAPI) infoForUser(ctx context.Context, tag names.UserTag, user coreuser.User) params.UserInfoResult {
	var lastLogin *time.Time
	if !user.LastLogin.IsZero() {
		lastLogin = &user.LastLogin
	}
	result := params.UserInfoResult{
		Result: &params.UserInfo{
			Username:       user.Name.Name(),
			DisplayName:    user.DisplayName,
			CreatedBy:      user.CreatorName.Name(),
			DateCreated:    user.CreatedAt,
			LastConnection: lastLogin,
			Disabled:       user.Disabled,
		},
	}

	if user.Disabled {
		// disabled users have no access to the controller.
		result.Result.Access = string(permission.NoAccess)
		return result
	}

	access, err := api.accessService.ReadUserAccessLevelForTarget(ctx, coreuser.NameFromTag(tag), permission.ID{
		ObjectType: permission.Controller,
		Key:        api.controllerUUID,
	})
	if err != nil && !errors.Is(err, accesserrors.AccessNotFound) {
		result.Result = nil
		result.Error = apiservererrors.ServerError(err)
	} else {
		result.Result.Access = string(access)
	}
	return result
}

func (api *UserManagerAPI) checkCanRead(ctx context.Context, modelTag names.Tag) error {
	return api.authorizer.HasPermission(ctx, permission.ReadAccess, modelTag)
}

// ModelUserInfo returns information on all users in the model.
func (api *UserManagerAPI) ModelUserInfo(ctx context.Context, args params.Entities) (params.ModelUserInfoResults, error) {
	var result params.ModelUserInfoResults
	for _, entity := range args.Entities {
		modelTag, err := names.ParseModelTag(entity.Tag)
		if err != nil {
			return result, errors.Trace(err)
		}
		infos, err := api.modelUserInfo(ctx, modelTag)
		if err != nil {
			return result, errors.Trace(err)
		}
		result.Results = append(result.Results, infos...)
	}
	return result, nil
}

func (api *UserManagerAPI) modelUserInfo(ctx context.Context, modelTag names.ModelTag) ([]params.ModelUserInfoResult, error) {
	var results []params.ModelUserInfoResult
	if err := api.checkCanRead(ctx, modelTag); err != nil {
		return results, err
	}

	// If the user is a controller superuser, they are considered a model
	// admin.
	modelUserInfo, err := commonmodel.ModelUserInfo(
		ctx,
		api.modelService,
		modelTag,
		api.apiUser.Name,
		api.isModelAdmin(ctx, modelTag),
	)
	if err != nil {
		return results, errors.Trace(err)
	}

	for i := range modelUserInfo {
		modelUserInfo[i].ModelTag = modelTag.String()
		results = append(results, params.ModelUserInfoResult{
			Result: &modelUserInfo[i],
		})
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
	if !api.isAdmin {
		userTag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		if _, err := api.hasControllerAdminAccess(ctx); err != nil && api.apiUserTag != userTag {
			return err
		}
	}

	if strings.TrimSpace(arg.Password) == "" {
		return errors.New("cannot use an empty password")
	}

	userTag, err := names.ParseUserTag(arg.Tag)
	if err != nil {
		return errors.Trace(err)
	}

	pass := auth.NewPassword(arg.Password)
	defer pass.Destroy()

	if err := api.accessService.SetPassword(ctx, coreuser.NameFromTag(userTag), pass); err != nil {
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

	isSuperUser, err := api.hasControllerAdminAccess(ctx)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return result, errors.Trace(err)
	}

	result.Results = make([]params.AddUserResult, len(args.Entities))
	for i, arg := range args.Entities {
		result.Results[i] = params.AddUserResult{Tag: arg.Tag}

		userTag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if isSuperUser && api.apiUserTag != userTag {
			key, err := api.accessService.ResetPassword(ctx, coreuser.NameFromTag(userTag))
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

// isModelAdmin checks if the user is a controller superuser or admin on the
// model.
func (api *UserManagerAPI) isModelAdmin(ctx context.Context, modelTag names.ModelTag) bool {
	if api.isAdmin {
		return true
	}
	return api.authorizer.HasPermission(ctx, permission.AdminAccess, modelTag) == nil
}
