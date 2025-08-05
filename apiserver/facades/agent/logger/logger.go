// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
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
	authorizer      facade.Authorizer
	watcherRegistry facade.WatcherRegistry

	modelConfigService ModelConfigService
}

var _ Logger = (*LoggerAPI)(nil)

// NewLoggerAPI returns a LoggerAPI facade.
func NewLoggerAPI(authorizer facade.Authorizer,
	watcherRegistry facade.WatcherRegistry,
	modelConfigService ModelConfigService) (*LoggerAPI, error) {
	if !authorizer.AuthMachineAgent() &&
		!authorizer.AuthUnitAgent() &&
		!authorizer.AuthApplicationAgent() &&
		!authorizer.AuthModelAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &LoggerAPI{
		authorizer:         authorizer,
		watcherRegistry:    watcherRegistry,
		modelConfigService: modelConfigService,
	}, nil
}

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
		if !api.authorizer.AuthOwner(tag) {
			result[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		// TODO(wallyworld) - only trigger on logging change
		watch, err := api.modelConfigService.Watch(ctx)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}

		notifyWatcher, err := watcher.Normalise[[]string](watch)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result[i].NotifyWatcherId, _, err = internal.EnsureRegisterWatcher[struct{}](ctx, api.watcherRegistry, notifyWatcher)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return params.NotifyWatchResults{Results: result}
}

// LoggingConfig reports the logging configuration for the agents specified.
func (api *LoggerAPI) LoggingConfig(ctx context.Context, arg params.Entities) params.StringResults {
	if len(arg.Entities) == 0 {
		return params.StringResults{}
	}
	results := make([]params.StringResult, len(arg.Entities))
	config, configErr := api.modelConfigService.ModelConfig(ctx)
	for i, entity := range arg.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !api.authorizer.AuthOwner(tag) {
			results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if configErr != nil {
			results[i].Error = apiservererrors.ServerError(configErr)
			continue
		}
		results[i].Result = config.LoggingConfig()
	}
	return params.StringResults{Results: results}
}
