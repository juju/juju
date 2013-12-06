// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentials

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
	"strings"
)

// Credentials defines the methods on the credentials API end point.
type Credentials interface {
	AuthorisedKeys(args params.Entities) params.StringsResults
	WatchAuthorisedKeys(args params.Entities) params.NotifyWatchResults
}

// CredentialsAPI implements the Credentials interface and is the concrete
// implementation of the api end point.
type CredentialsAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ Credentials = (*CredentialsAPI)(nil)

// NewCredentialsAPI creates a new server-side credentials API end point.
func NewCredentialsAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*CredentialsAPI, error) {
	if !authorizer.AuthStateManager() {
		return nil, common.ErrPerm
	}
	return &CredentialsAPI{state: st, resources: resources, authorizer: authorizer}, nil
}

// WatchAuthorisedKeys starts a watcher to track changes to the authorised ssh keys
// for the specified machines.
// The current implementation relies on global authorised keys being stored in the environment config.
// This will change as new user management and authorisation functionality is added.
func (api *CredentialsAPI) WatchAuthorisedKeys(arg params.Entities) params.NotifyWatchResults {
	results := make([]params.NotifyWatchResult, len(arg.Entities))

	if !api.authorizer.AuthStateManager() {
		for i, _ := range arg.Entities {
			results[i].Error = common.ServerError(common.ErrPerm)
		}
		return params.NotifyWatchResults{results}
	}

	// For now, authorised keys are global, common to all machines, so
	// we don't use the machine except to verify it exists.
	for i, entity := range arg.Entities {
		if _, err := api.state.FindEntity(entity.Tag); err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		var err error
		watch := api.state.WatchForEnvironConfigChanges()
		// Consume the initial event.
		if _, ok := <-watch.Changes(); ok {
			results[i].NotifyWatcherId = api.resources.Register(watch)
			err = nil
		} else {
			err = watcher.MustErr(watch)
		}
		results[i].Error = common.ServerError(err)
	}
	return params.NotifyWatchResults{results}
}

// AuthorisedKeys reports the authorised ssh keys for the specified machines.
// The current implementation relies on global authorised keys being stored in the environment config.
// This will change as new user management and authorisation functionality is added.
func (api *CredentialsAPI) AuthorisedKeys(arg params.Entities) params.StringsResults {
	if len(arg.Entities) == 0 {
		return params.StringsResults{}
	}
	results := make([]params.StringsResult, len(arg.Entities))

	if !api.authorizer.AuthStateManager() {
		for i, _ := range arg.Entities {
			results[i].Error = common.ServerError(common.ErrPerm)
		}
		return params.StringsResults{results}
	}

	config, configErr := api.state.EnvironConfig()
	// For now, authorised keys are global, common to all machines, so
	// we don't use the machine except to verify it exists.
	for i, entity := range arg.Entities {
		if _, err := api.state.FindEntity(entity.Tag); err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		var err error
		if configErr == nil {
			keysString := config.AuthorizedKeys()
			keys := strings.Split(keysString, "\n")
			results[i].Result = keys
			err = nil
		} else {
			err = configErr
		}
		results[i].Error = common.ServerError(err)
	}
	return params.StringsResults{results}
}
