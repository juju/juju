// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"github.com/juju/loggo"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
)

var logger = loggo.GetLogger("juju.api.logger")

// Logger defines the methods on the logger API end point.  Unfortunately, the
// api infrastructure doesn't allow interfaces to be used as an actual
// endpoint because our rpc mechanism panics.  However, I still feel that this
// provides a useful documentation purpose.
type Logger interface {
	WatchLoggingConfig(args params.Entities) params.NotifyWatchResults
	LoggingConfig(args params.Entities) params.StringResults
}

// LoggerAPI implements the Logger interface and is the concrete
// implementation of the api end point.
type LoggerAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ Logger = (*LoggerAPI)(nil)

// NewLoggerAPI creates a new server-side logger API end point.
func NewLoggerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*LoggerAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	return &LoggerAPI{state: st, resources: resources, authorizer: authorizer}, nil
}

// WatchLoggingConfig starts a watcher to track changes to the logging config
// for the agents specified..  Unfortunately the current infrastruture makes
// watching parts of the config non-trivial, so currently any change to the
// config will cause the watcher to notify the client.
func (api *LoggerAPI) WatchLoggingConfig(arg params.Entities) params.NotifyWatchResults {
	result := make([]params.NotifyWatchResult, len(arg.Entities))
	for i, entity := range arg.Entities {
		err := common.ErrPerm
		if api.authorizer.AuthOwner(entity.Tag) {
			watch := api.state.WatchForEnvironConfigChanges()
			// Consume the initial event. Technically, API calls to Watch
			// 'transmit' the initial event in the Watch response. But
			// NotifyWatchers have no state to transmit.
			if _, ok := <-watch.Changes(); ok {
				result[i].NotifyWatcherId = api.resources.Register(watch)
				err = nil
			} else {
				err = watcher.MustErr(watch)
			}
		}
		result[i].Error = common.ServerError(err)
	}
	return params.NotifyWatchResults{result}
}

// LoggingConfig reports the logging configuration for the agents specified.
func (api *LoggerAPI) LoggingConfig(arg params.Entities) params.StringResults {
	if len(arg.Entities) == 0 {
		return params.StringResults{}
	}
	results := make([]params.StringResult, len(arg.Entities))
	config, configErr := api.state.EnvironConfig()
	for i, entity := range arg.Entities {
		err := common.ErrPerm
		if api.authorizer.AuthOwner(entity.Tag) {
			if configErr == nil {
				results[i].Result = config.LoggingConfig()
				err = nil
			} else {
				err = configErr
			}
		}
		results[i].Error = common.ServerError(err)
	}
	return params.StringResults{results}
}
