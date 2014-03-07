// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"fmt"

	"github.com/loggo/loggo"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

var logger = loggo.GetLogger("juju.state.apiserver.usermanager")

// UserManager defines the methods on the usermanager API end point.
type UserManager interface {
	AddUser(arg params.ModifyUsers) (params.ErrorResults, error)
	RemoveUser(arg params.ModifyUsers) (params.ErrorResults, error)
}

// UserManagerAPI implements the user manager interface and is the concrete
// implementation of the api end point.
type UserManagerAPI struct {
	state       *state.State
	authorizer  common.Authorizer
	getCanWrite common.GetAuthFunc
}

var _ UserManager = (*UserManagerAPI)(nil)

func NewUserManagerAPI(
	st *state.State,
	authorizer common.Authorizer,
) (*UserManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	// TODO(mattyw) - replace stub with real canRead function
	// For now, only admins can add users.
	getCanWrite := common.AuthAlways(true)
	return &UserManagerAPI{
			state:       st,
			authorizer:  authorizer,
			getCanWrite: getCanWrite},
		nil
}

func (api *UserManagerAPI) AddUser(args params.ModifyUsers) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Params)),
	}
	canWrite, err := api.getCanWrite()
	if err != nil {
		result.Results[0].Error = common.ServerError(err)
		return result, err
	}
	for i, arg := range args.Params {
		if !canWrite(arg.Tag) {
			result.Results[0].Error = common.ServerError(common.ErrPerm)
			continue
		}
		_, err = api.state.AddUser(arg.Tag, arg.Password)
		if err != nil {
			result.Results[i].Error = common.ServerError(fmt.Errorf("Failed to create user: %s", err))
			continue
		}
	}
	return result, nil
}

func (api *UserManagerAPI) RemoveUser(args params.ModifyUsers) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Params)),
	}
	canWrite, err := api.getCanWrite()
	if err != nil {
		result.Results[0].Error = common.ServerError(err)
		return result, err
	}
	for i, arg := range args.Params {
		if !canWrite(arg.Tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		user, err := api.state.User(arg.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(fmt.Errorf("Failed to find user %s: %s", arg.Tag, err))
			continue
		}
		err = user.SetInactive()
		if err != nil {
			result.Results[i].Error = common.ServerError(fmt.Errorf("Failed to remove user: %s", err))
			continue
		}
	}
	return result, nil
}
