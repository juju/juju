// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.usermanager")

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
	state      *state.State
	authorizer common.Authorizer
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

	return &UserManagerAPI{
		state:      st,
		authorizer: authorizer,
	}, nil
}

// AddUser adds a user.
func (api *UserManagerAPI) AddUser(args ModifyUsers) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}
	user, err := api.getLoggedInUser()
	if err != nil {
		return result, errors.Wrap(err, common.ErrPerm)
	}
	for i, arg := range args.Changes {
		username := arg.Username
		if username == "" {
			username = arg.Tag
		}
		_, err := api.state.AddUser(username, arg.DisplayName, arg.Password, user.Id())
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
	for i, arg := range args.Entities {
		if !names.IsValidUserName(arg.Tag) {
			result.Results[i].Error = common.ServerError(errors.Errorf("%q is not a valid username", arg.Tag))
			continue
		}
		user, err := api.state.User(names.NewLocalUserTag(arg.Tag))
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = user.Deactivate()
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Errorf("Failed to remove user: %s", err))
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

	for i, userArg := range args.Entities {
		tag, err := names.ParseUserTag(userArg.Tag)
		if err != nil {
			results.Results[0].Error = common.ServerError(err)
			continue
		}
		user, err := api.state.User(tag)
		var result UserInfoResult
		if err != nil {
			if errors.IsNotFound(err) {
				result.Error = common.ServerError(common.ErrPerm)
			} else {
				result.Error = common.ServerError(err)
			}
		} else {
			info := UserInfo{
				Username:       tag.Name(),
				DisplayName:    user.DisplayName(),
				CreatedBy:      user.CreatedBy(),
				DateCreated:    user.DateCreated(),
				LastConnection: user.LastLogin(),
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
	for i, arg := range args.Changes {
		loggedInUser, err := api.getLoggedInUser()
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		username := arg.Username
		if username == "" {
			username = arg.Tag
		}

		if !names.IsValidUserName(username) {
			result.Results[i].Error = common.ServerError(errors.Errorf("%q is not a valid username", arg.Tag))
			continue
		}
		searchTag := names.NewLocalUserTag(username)
		if loggedInUser != searchTag {
			result.Results[i].Error = common.ServerError(errors.Errorf("can only change the password of the current user (%s)", loggedInUser.Id()))
			continue
		}

		argUser, err := api.state.User(searchTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Annotate(err, "failed to find user"))
			continue
		}

		err = argUser.SetPassword(arg.Password)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Annotate(err, "failed to set password"))
			continue
		}
	}
	return result, nil
}

func (api *UserManagerAPI) getLoggedInUser() (names.UserTag, error) {
	switch tag := api.authorizer.GetAuthTag().(type) {
	case names.UserTag:
		return tag, nil
	default:
		return names.UserTag{}, errors.New("authorizer not a user")
	}
}
