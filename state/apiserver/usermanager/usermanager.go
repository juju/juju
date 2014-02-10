// Copyright 2013 Canonical Ltd.
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
	resources   *common.Resources
	authorizer  common.Authorizer
	getCanRead  common.GetAuthFunc
	getCanWrite common.GetAuthFunc
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

	// TODO(mattyw) - replace stub with real canRead function
	// For now, only admins can read users.
	// Copied from the keymanager.go
	getCanRead := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return authorizer.GetAuthTag() == "user-admin"
		}, nil
	}
	// TODO(mattyw) - replace stub with real canRead function
	// For now, only admins can add users.
	getCanWrite := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			if _, err := st.User(tag); err != nil {
				return false
			}
			return authorizer.GetAuthTag() == "user-admin"
		}, nil
	}
	return &UserManagerAPI{
			state:       st,
			resources:   resources,
			authorizer:  authorizer,
			getCanRead:  getCanRead,
			getCanWrite: getCanWrite},
		nil
}

func (api *UserManagerAPI) AddUser(args params.ModifyUsers) (params.ErrorResults, error) {
	_, err := api.state.AddUser(args.Tag, args.Password)
	if err != nil {
		return params.ErrorResults{
			[]params.ErrorResult{
				{Error: common.ServerError(fmt.Errorf("Failed to create user: %s", err))},
			},
		}, err
	}
	return params.ErrorResults{}, nil
}

func (api *UserManagerAPI) RemoveUser(args params.ModifyUsers) (params.ErrorResults, error) {
	return params.ErrorResults{}, nil
}
