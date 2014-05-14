// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"fmt"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

var logger = loggo.GetLogger("juju.state.apiserver.usermanager")

// UserManager defines the methods on the usermanager API end point.
type UserManager interface {
	AddUser(arg params.EntityPasswords) (params.ErrorResults, error)
	RemoveUser(arg params.Entities) (params.ErrorResults, error)
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

	// TODO(mattyw) - replace stub with real canWrite function
	getCanWrite := common.AuthAlways(true)
	return &UserManagerAPI{
			state:       st,
			authorizer:  authorizer,
			getCanWrite: getCanWrite},
		nil
}

func (api *UserManagerAPI) AddUser(args params.EntityPasswords) (params.ErrorResults, error) {
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
	for i, arg := range args.Changes {
		if !canWrite(arg.Tag) {
			result.Results[0].Error = common.ServerError(common.ErrPerm)
			continue
		}
		_, err := api.state.AddUser(arg.Tag, arg.Password)
		if err != nil {
			err = fmt.Errorf("Failed to create user: %v", err)
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}

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
