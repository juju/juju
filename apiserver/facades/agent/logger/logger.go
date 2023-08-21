// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// Logger defines the methods on the logger API end point.  Unfortunately, the
// api infrastructure doesn't allow interfaces to be used as an actual
// endpoint because our rpc mechanism panics.  However, I still feel that this
// provides a useful documentation purpose.
type Logger interface {
	WatchLoggingConfig(ctx context.Context, args params.Entities) params.NotifyWatchResults
	LoggingConfig(ctx context.Context, args params.Entities) params.StringResults
}

// LoggerAPI implements the Logger interface and is the concrete
// implementation of the api end point.
type LoggerAPI struct {
	model      *state.Model
	resources  facade.Resources
	authorizer facade.Authorizer
}

var _ Logger = (*LoggerAPI)(nil)

// WatchLoggingConfig starts a watcher to track changes to the logging config
// for the agents specified..  Unfortunately the current infrastructure makes
// watching parts of the config non-trivial, so currently any change to the
// config will cause the watcher to notify the client.
func (api *LoggerAPI) WatchLoggingConfig(ctx context.Context, arg params.Entities) params.NotifyWatchResults {
	result := make([]params.NotifyWatchResult, len(arg.Entities))
	for i, entity := range arg.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = apiservererrors.ErrPerm
		if api.authorizer.AuthOwner(tag) {
			// TODO(wallyworld) - only trigger on logging change
			watch := api.model.WatchForModelConfigChanges()
			// Consume the initial event. Technically, API calls to Watch
			// 'transmit' the initial event in the Watch response. But
			// NotifyWatchers have no state to transmit.
			if _, ok := <-watch.Changes(); ok {
				result[i].NotifyWatcherId = api.resources.Register(watch)
				err = nil
			} else {
				err = errors.New("programming error: channel should not be closed")
			}
		}
		result[i].Error = apiservererrors.ServerError(err)
	}
	return params.NotifyWatchResults{Results: result}
}

// LoggingConfig reports the logging configuration for the agents specified.
func (api *LoggerAPI) LoggingConfig(ctx context.Context, arg params.Entities) params.StringResults {
	if len(arg.Entities) == 0 {
		return params.StringResults{}
	}
	results := make([]params.StringResult, len(arg.Entities))
	config, configErr := api.model.ModelConfig(ctx)
	for i, entity := range arg.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = apiservererrors.ErrPerm
		if api.authorizer.AuthOwner(tag) {
			if configErr == nil {
				results[i].Result = config.LoggingConfig()
				err = nil
			} else {
				err = configErr
			}
		}
		results[i].Error = apiservererrors.ServerError(err)
	}
	return params.StringResults{Results: results}
}
