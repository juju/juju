// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/utils/ssh"
)

// KeyUpdater defines the methods on the keyupdater API end point.
type KeyUpdater interface {
	AuthorisedKeys(args params.Entities) (params.StringsResults, error)
	WatchAuthorisedKeys(args params.Entities) (params.NotifyWatchResults, error)
}

// KeyUpdaterAPI implements the KeyUpdater interface and is the concrete
// implementation of the api end point.
type KeyUpdaterAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
	getCanRead common.GetAuthFunc
}

var _ KeyUpdater = (*KeyUpdaterAPI)(nil)

// NewKeyUpdaterAPI creates a new server-side keyupdater API end point.
func NewKeyUpdaterAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*KeyUpdaterAPI, error) {
	// Only machine agents have access to the keyupdater service.
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	// No-one else except the machine itself can only read a machine's own credentials.
	getCanRead := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &KeyUpdaterAPI{state: st, resources: resources, authorizer: authorizer, getCanRead: getCanRead}, nil
}

// WatchAuthorisedKeys starts a watcher to track changes to the authorised ssh keys
// for the specified machines.
// The current implementation relies on global authorised keys being stored in the environment config.
// This will change as new user management and authorisation functionality is added.
func (api *KeyUpdaterAPI) WatchAuthorisedKeys(arg params.Entities) (params.NotifyWatchResults, error) {
	results := make([]params.NotifyWatchResult, len(arg.Entities))

	canRead, err := api.getCanRead()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range arg.Entities {
		// 1. Check permissions
		if !canRead(entity.Tag) {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		// 2. Check entity exists
		if _, err := api.state.FindEntity(entity.Tag); err != nil {
			if errors.IsNotFoundError(err) {
				results[i].Error = common.ServerError(common.ErrPerm)
			} else {
				results[i].Error = common.ServerError(err)
			}
			continue
		}
		// 3. Watch fr changes
		var err error
		watch := api.state.WatchForEnvironConfigChanges()
		// Consume the initial event.
		if _, ok := <-watch.Changes(); ok {
			results[i].NotifyWatcherId = api.resources.Register(watch)
		} else {
			err = watcher.MustErr(watch)
		}
		results[i].Error = common.ServerError(err)
	}
	return params.NotifyWatchResults{results}, nil
}

// AuthorisedKeys reports the authorised ssh keys for the specified machines.
// The current implementation relies on global authorised keys being stored in the environment config.
// This will change as new user management and authorisation functionality is added.
func (api *KeyUpdaterAPI) AuthorisedKeys(arg params.Entities) (params.StringsResults, error) {
	if len(arg.Entities) == 0 {
		return params.StringsResults{}, nil
	}
	results := make([]params.StringsResult, len(arg.Entities))

	// For now, authorised keys are global, common to all machines.
	var keys []string
	config, configErr := api.state.EnvironConfig()
	if configErr == nil {
		keys = ssh.SplitAuthorisedKeys(config.AuthorizedKeys())
	}

	canRead, err := api.getCanRead()
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range arg.Entities {
		// 1. Check permissions
		if !canRead(entity.Tag) {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		// 2. Check entity exists
		if _, err := api.state.FindEntity(entity.Tag); err != nil {
			if errors.IsNotFoundError(err) {
				results[i].Error = common.ServerError(common.ErrPerm)
			} else {
				results[i].Error = common.ServerError(err)
			}
			continue
		}
		// 3. Get keys
		var err error
		if configErr == nil {
			results[i].Result = keys
		} else {
			err = configErr
		}
		results[i].Error = common.ServerError(err)
	}
	return params.StringsResults{results}, nil
}
