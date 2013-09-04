// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"errors"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
	agenttools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

// LoggerAPI defines the methods on the logger API end point.
type LoggerAPI interface {
	WatchLoggingConfig(args params.Entity) params.NotifyWatchResult
	LoggingConfig(args params.Entity) params.StringResult
}

// NewLoggerAPI creates a new server-side logger API end point.
func NewLoggerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (LoggerAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	return &loggerAPI{st: st, resources: resources, authorizer: authorizer}, nil
}

type loggerAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ LoggerAPI = (*loggerAPI)(nil)

// WatchLoggingConfig starts a watcher to track changes to the logging config.
// Unfortunately the current infrastruture makes watching parts of the config
// non-trivial, so currently any change to the config will cause the watcher
// to notify the client.
func (api *loggerAPI) WatchLoggingConfig(arg params.Entities) params.NotifyWatchResults {
	result := make([]param.NotifyWatchResult, len(arg.Entities))
	for i, entity := range arg.Entities {
		err := common.ErrPerm
		if u.authorizer.AuthOwner(entity.Tag) {
			watch := api.state.WatchForEnvironConfigChanges()
			// Consume the initial event. Technically, API calls to Watch
			// 'transmit' the initial event in the Watch response. But
			// NotifyWatchers have no state to transmit.
			if _, ok := <-watch.Changes(); ok {
				result[i].NotifyWatcherId = u.resources.Register(watch)
				err = nil
			} else {
				err = watcher.MustErr(watch)
			}
		}
		result[i].Error = common.ServerError(err)
	}
	return params.NotifyWatchResults{result}
}

// DesiredVersion reports the Agent Version that we want that agent to be running
func (api *loggerAPI) LoggingConfig(arg params.Entities) params.StringResults {
	results := make([]param.StringResult, len(arg.Entities))
	// If someone is stupid enough to call this function with zero entities,
	// lets punish them by making them wait for us to get the environ config
	// from state.
	config, configErr := api.state.EnvironConfig()
	for i, entity := range arg.Entities {
		err := common.ErrPerm
		if u.authorizer.AuthOwner(entity.Tag) {
			if configErr != nil {
				results[i].Result = config.LoggingConfig()
				err = nil
			} else {
				err = configErr
			}
		}
		results[i].Error = common.ServerError(err)
	}
	return param.StringResults{result}
}
