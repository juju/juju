// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"time"

	"gopkg.in/macaroon.v1"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.usermanager")

func init() {
	common.RegisterStandardFacade("UserManager", 1, NewUserManagerAPI)
}

// UserManagerAPI implements the user manager interface and is the concrete
// implementation of the api end point.
type UserManagerAPI struct {
	state                    *state.State
	authorizer               common.Authorizer
	createLocalLoginMacaroon func(names.UserTag) (*macaroon.Macaroon, error)
	check                    *common.BlockChecker
}

func NewUserManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*UserManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	resource, ok := resources.Get("createLocalLoginMacaroon").(common.ValueResource)
	if !ok {
		return nil, errors.NotFoundf("userAuth resource")
	}
	createLocalLoginMacaroon, ok := resource.Value.(func(names.UserTag) (*macaroon.Macaroon, error))
	if !ok {
		return nil, errors.NotValidf("userAuth resource")
	}

	return &UserManagerAPI{
		state:                    st,
		authorizer:               authorizer,
		createLocalLoginMacaroon: createLocalLoginMacaroon,
		check: common.NewBlockChecker(st),
	}, nil
}

func (api *UserManagerAPI) permissionCheck(user names.UserTag) error {
	// TODO(thumper): PERMISSIONS Change this permission check when we have
	// real permissions. For now, only the owner of the initial environment is
	// able to add users.
	initialEnv, err := api.state.ControllerModel()
	if err != nil {
		return errors.Trace(err)
	}
	if user != initialEnv.Owner() {
		return errors.Trace(common.ErrPerm)
	}
	return nil
}

// AddUser adds a user with a username, and either a password or
// a randomly generated secret key which will be returned.
func (api *UserManagerAPI) AddUser(args params.AddUsers) (params.AddUserResults, error) {
	result := params.AddUserResults{
		Results: make([]params.AddUserResult, len(args.Users)),
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}

	if len(args.Users) == 0 {
		return result, nil
	}
	loggedInUser, err := api.getLoggedInUser()
	if err != nil {
		return result, errors.Wrap(err, common.ErrPerm)
	}
	// TODO(thumper): PERMISSIONS Change this permission check when we have
	// real permissions. For now, only the owner of the initial model is
	// able to add users.
	if err := api.permissionCheck(loggedInUser); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Users {
		var user *state.User
		if arg.Password != "" {
			user, err = api.state.AddUser(arg.Username, arg.DisplayName, arg.Password, loggedInUser.Id())
		} else {
			user, err = api.state.AddUserWithSecretKey(arg.Username, arg.DisplayName, loggedInUser.Id())
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

		if len(arg.SharedModelTags) > 0 {
			modelAccess, err := modelmanager.FromModelAccessParam(arg.ModelAccess)
			if err != nil {
				err = errors.Annotatef(err, "user %q created but models not shared", arg.Username)
				result.Results[i].Error = common.ServerError(err)
				continue
			}
			userTag := user.Tag().(names.UserTag)
			for _, modelTagStr := range arg.SharedModelTags {
				modelTag, err := names.ParseModelTag(modelTagStr)
				if err != nil {
					err = errors.Annotatef(err, "user %q created but model %q not shared", arg.Username, modelTagStr)
					result.Results[i].Error = common.ServerError(err)
					break
				}
				err = modelmanager.ChangeModelAccess(api.state, modelTag, loggedInUser, userTag, params.GrantModelAccess, modelAccess)
				if err != nil {
					err = errors.Annotatef(err, "user %q created but model %q not shared", arg.Username, modelTagStr)
					result.Results[i].Error = common.ServerError(err)
					break
				}
			}
		}
	}
	return result, nil
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
// the action is consided a success.
func (api *UserManagerAPI) EnableUser(users params.Entities) (params.ErrorResults, error) {
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return api.enableUserImpl(users, "enable", (*state.User).Enable)
}

// DisableUser disables one or more users.  If the user is already disabled,
// the action is consided a success.
func (api *UserManagerAPI) DisableUser(users params.Entities) (params.ErrorResults, error) {
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return api.enableUserImpl(users, "disable", (*state.User).Disable)
}

func (api *UserManagerAPI) enableUserImpl(args params.Entities, action string, method func(*state.User) error) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	loggedInUser, err := api.getLoggedInUser()
	if err != nil {
		return result, errors.Wrap(err, common.ErrPerm)
	}
	// TODO(thumper): PERMISSIONS Change this permission check when we have
	// real permissions. For now, only the owner of the initial environment is
	// able to add users.
	if err := api.permissionCheck(loggedInUser); err != nil {
		return result, errors.Trace(err)
	}

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
		return params.UserInfoResult{
			Result: &params.UserInfo{
				Username:       user.Name(),
				DisplayName:    user.DisplayName(),
				CreatedBy:      user.CreatedBy(),
				DateCreated:    user.DateCreated(),
				LastConnection: lastLogin,
				Disabled:       user.IsDisabled(),
			},
		}
	}

	argCount := len(request.Entities)
	if argCount == 0 {
		users, err := api.state.AllUsers(request.IncludeDisabled)
		if err != nil {
			return results, errors.Trace(err)
		}
		for _, user := range users {
			results.Results = append(results.Results, infoForUser(user))
		}
		return results, nil
	}

	results.Results = make([]params.UserInfoResult, argCount)
	for i, arg := range request.Entities {
		user, err := api.getUser(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i] = infoForUser(user)
	}

	return results, nil
}

// SetPassword changes the stored password for the specified users.
func (api *UserManagerAPI) SetPassword(args params.EntityPasswords) (params.ErrorResults, error) {
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}
	loggedInUser, err := api.getLoggedInUser()
	if err != nil {
		return result, common.ErrPerm
	}
	adminUser, err := api.checkAdminUser(loggedInUser)
	if err != nil {
		return result, common.ServerError(err)
	}
	for i, arg := range args.Changes {
		if err := api.setPassword(loggedInUser, arg, adminUser); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

func (api *UserManagerAPI) setPassword(loggedInUser names.UserTag, arg params.EntityPassword, adminUser bool) error {
	user, err := api.getUser(arg.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	if loggedInUser != user.UserTag() && !adminUser {
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

// CreateLocalLoginMacaroon creates a macaroon for the specified users to use
// for future logins.
func (api *UserManagerAPI) CreateLocalLoginMacaroon(args params.Entities) (params.MacaroonResults, error) {
	results := params.MacaroonResults{
		Results: make([]params.MacaroonResult, len(args.Entities)),
	}
	loggedInUser, err := api.getLoggedInUser()
	if err != nil {
		return results, common.ErrPerm
	}
	adminUser, err := api.checkAdminUser(loggedInUser)
	if err != nil {
		return results, common.ServerError(err)
	}
	createLocalLoginMacaroon := func(arg params.Entity) (*macaroon.Macaroon, error) {
		user, err := api.getUser(arg.Tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if loggedInUser != user.UserTag() && !adminUser {
			return nil, errors.Trace(common.ErrPerm)
		}
		return api.createLocalLoginMacaroon(user.UserTag())
	}
	for i, arg := range args.Entities {
		m, err := createLocalLoginMacaroon(arg)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = m
	}
	return results, nil
}

func (api *UserManagerAPI) checkAdminUser(userTag names.UserTag) (bool, error) {
	err := api.permissionCheck(userTag)
	switch errors.Cause(err) {
	case nil:
		return true, nil
	case common.ErrPerm:
		return false, nil
	}
	return false, errors.Trace(err)
}

func (api *UserManagerAPI) getLoggedInUser() (names.UserTag, error) {
	switch tag := api.authorizer.GetAuthTag().(type) {
	case names.UserTag:
		return tag, nil
	default:
		return names.UserTag{}, errors.New("authorizer not a user")
	}
}
