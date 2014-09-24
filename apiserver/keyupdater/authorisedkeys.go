// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/utils/ssh"
)

func init() {
	common.RegisterStandardFacade("KeyUpdater", 0, NewKeyUpdaterAPI)
}

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
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		// 1. Check permissions
		if !canRead(tag) {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		// 2. Check entity exists
		if _, err := api.state.FindEntity(tag); err != nil {
			if errors.IsNotFound(err) {
				results[i].Error = common.ServerError(common.ErrPerm)
			} else {
				results[i].Error = common.ServerError(err)
			}
			continue
		}
		// 3. Watch for changes
		watch := api.state.WatchForEnvironConfigChanges()
		// Consume the initial event.
		if _, ok := <-watch.Changes(); ok {
			results[i].NotifyWatcherId = api.resources.Register(watch)
		} else {
			err = watcher.EnsureErr(watch)
		}
		results[i].Error = common.ServerError(err)
	}
	return params.NotifyWatchResults{Results: results}, nil
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
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		// 1. Check permissions
		if !canRead(tag) {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		// 2. Check entity exists
		if _, err := api.state.FindEntity(tag); err != nil {
			if errors.IsNotFound(err) {
				results[i].Error = common.ServerError(common.ErrPerm)
			} else {
				results[i].Error = common.ServerError(err)
			}
			continue
		}
		// 3. Get keys
		if configErr == nil {
			results[i].Result = keys
		} else {
			err = configErr
		}
		results[i].Error = common.ServerError(err)
	}
	return params.StringsResults{Results: results}, nil
}
