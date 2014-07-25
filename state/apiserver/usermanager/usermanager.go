// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
)

var logger = loggo.GetLogger("juju.state.apiserver.usermanager")

func init() {
	common.RegisterStandardFacade("UserManager", 0, NewUserManagerAPI)
}

// UserManager defines the methods on the usermanager API end point.
type UserManager interface {
	AddUser(arg ModifyUsers) (params.ErrorResults, error)
	RemoveUser(arg params.Entities) (params.ErrorResults, error)
	SetPassword(args ModifyUsers) (params.ErrorResults, error)
}

// UserInfo holds information on a user.
type UserInfo struct {
	Username       string     `json:username`
	DisplayName    string     `json:display-name`
	CreatedBy      string     `json:created-by`
	DateCreated    time.Time  `json:date-created`
	LastConnection *time.Time `json:last-connection`
}

// UserInfoResult holds the result of a UserInfo call.
type UserInfoResult struct {
	Result *UserInfo     `json:result,omitempty`
	Error  *params.Error `json:error,omitempty`
}

// UserInfoResults holds the result of a bulk UserInfo API call.
type UserInfoResults struct {
	Results []UserInfoResult
}

// ModifyUsers holds the parameters for making a UserManager Add or Modify calls.
type ModifyUsers struct {
	Changes []ModifyUser
}

// ModifyUser stores the parameters used for a UserManager.Add|Remove call.
type ModifyUser struct {
	// Tag is here purely for backwards compatability. Older clients will
	// attempt to use the EntityPassword structure, so we need a Tag here
	// (which will be treated as Username)
	Tag         string
	Username    string
	DisplayName string
	Password    string
}

// UserManagerAPI implements the user manager interface and is the concrete
// implementation of the api end point.
type UserManagerAPI struct {
	state       *state.State
	authorizer  common.Authorizer
	getCanWrite common.GetAuthFunc
	getCanRead  common.GetAuthFunc
}

var _ UserManager = (*UserManagerAPI)(nil)

func NewUserManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*UserManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	// TODO(mattyw) - replace stub with real canWrite function
	getCanWrite := common.AuthAlways(true)

	// TODO(waigani) - replace stub with real canRead function
	getCanRead := common.AuthAlways(true)
	return &UserManagerAPI{
			state:       st,
			authorizer:  authorizer,
			getCanWrite: getCanWrite,
			getCanRead:  getCanRead},
		nil
}

// AddUser adds a user.
func (api *UserManagerAPI) AddUser(args ModifyUsers) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}
	canWrite, err := api.getCanWrite()
	if err != nil {
		result.Results[0].Error = common.ServerError(err)
		return result, err
	}
	user := api.getLoggedInUser()
	if user == nil {
		return result, fmt.Errorf("api connection is not through a user")
	}
	for i, arg := range args.Changes {
		if !canWrite(arg.Tag) {
			result.Results[0].Error = common.ServerError(common.ErrPerm)
			continue
		}
		username := arg.Username
		if username == "" {
			username = arg.Tag
		}
		_, err := api.state.AddUser(username, arg.DisplayName, arg.Password, user.Name())
		if err != nil {
			err = errors.Annotate(err, "failed to create user")
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}

// RemoveUser removes a user.
func (api *UserManagerAPI) RemoveUser(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWrite, err := api.getCanWrite()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		if !canWrite(arg.Tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		user, err := api.state.User(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = user.Deactivate()
		if err != nil {
			result.Results[i].Error = common.ServerError(fmt.Errorf("Failed to remove user: %s", err))
			continue
		}
	}
	return result, nil
}

// UserInfo returns information on a user.
func (api *UserManagerAPI) UserInfo(args params.Entities) (UserInfoResults, error) {
	results := UserInfoResults{
		Results: make([]UserInfoResult, len(args.Entities)),
	}

	canRead, err := api.getCanRead()
	if err != nil {
		return results, err
	}
	for i, userArg := range args.Entities {
		if !canRead(userArg.Tag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		tag, err := names.ParseUserTag(userArg.Tag)
		if err != nil {
			results.Results[0].Error = common.ServerError(err)
			continue
		}
		username := tag.Id()

		user, err := api.state.User(username)
		var result UserInfoResult
		if err != nil {
			if errors.IsNotFound(err) {
				result.Error = common.ServerError(common.ErrPerm)
			} else {
				result.Error = common.ServerError(err)
			}
		} else {
			info := UserInfo{
				Username:       username,
				DisplayName:    user.DisplayName(),
				CreatedBy:      user.CreatedBy(),
				DateCreated:    user.DateCreated(),
				LastConnection: user.LastConnection(),
			}
			result.Result = &info
		}
		results.Results[i] = result
	}

	return results, nil
}

func (api *UserManagerAPI) SetPassword(args ModifyUsers) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}
	canWrite, err := api.getCanWrite()
	if err != nil {
		return result, err
	}
	for i, arg := range args.Changes {
		if !canWrite(arg.Tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		username := arg.Username
		if username == "" {
			username = arg.Tag
		}

		argUser, err := api.state.User(username)
		if err != nil {
			result.Results[i].Error = common.ServerError(fmt.Errorf("Failed to find user %v", err))
			continue
		}

		loggedInUser := api.getLoggedInUser()
		if loggedInUser.Tag() != argUser.Tag() {
			result.Results[i].Error = common.ServerError(fmt.Errorf("Can only change the password of the current user (%s)", loggedInUser.Name()))
			continue
		}

		err = argUser.SetPassword(arg.Password)
		if err != nil {
			result.Results[i].Error = common.ServerError(fmt.Errorf("Failed to set password %v", err))
			continue
		}
	}
	return result, nil
}

func (api *UserManagerAPI) getLoggedInUser() *state.User {
	entity := api.authorizer.GetAuthEntity()
	if user, ok := entity.(*state.User); ok {
		return user
	}
	return nil
}
