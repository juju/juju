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
	loggingerrors "github.com/juju/juju/domain/logging/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Logger defines the methods on the logger API end point.  Unfortunately, the
// api infrastructure doesn't allow interfaces to be used as an actual
// endpoint because our rpc mechanism panics.  However, I still feel that this
// provides a useful documentation purpose.
type Logger interface {
	// WatchLoggingConfig starts watchers for model logging-config changes for
	// the requested agent entities.
	WatchLoggingConfig(ctx context.Context, args params.Entities) params.NotifyWatchResults

	// LoggingConfig reports the model logging-config value for the requested
	// agent entities.
	LoggingConfig(ctx context.Context, args params.Entities) params.StringResults
}

// LoggerV2 defines the methods on the v2 logger API end point.
type LoggerV2 interface {
	Logger

	// GetControllerLokiConfig reports the controller-wide Loki configuration for
	// the requested agent entity.
	GetControllerLokiConfig(ctx context.Context, args params.Entity) params.LokiConfigResult

	// WatchControllerLokiConfig starts a watcher for controller-wide Loki
	// configuration changes for the requested agent entity.
	WatchControllerLokiConfig(ctx context.Context, args params.Entity) params.NotifyWatchResult
}

// LoggerAPI implements the Logger interface and is the concrete
// implementation of the api end point.
type LoggerAPI struct {
	authorizer      facade.Authorizer
	watcherRegistry facade.WatcherRegistry

	modelConfigService ModelConfigService
}

var _ Logger = (*LoggerAPI)(nil)

// LoggerAPIV2 implements version 2 of the Logger facade.
type LoggerAPIV2 struct {
	*LoggerAPI

	controllerLokiConfigService ControllerLokiConfigService
}

var _ LoggerV2 = (*LoggerAPIV2)(nil)

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

// NewLoggerAPIV2 returns a v2 LoggerAPI facade.
func NewLoggerAPIV2(authorizer facade.Authorizer,
	watcherRegistry facade.WatcherRegistry,
	modelConfigService ModelConfigService,
	controllerLokiConfigService ControllerLokiConfigService) (*LoggerAPIV2, error) {
	loggerAPI, err := NewLoggerAPI(authorizer, watcherRegistry, modelConfigService)
	if err != nil {
		return nil, err
	}
	return &LoggerAPIV2{
		LoggerAPI:                   loggerAPI,
		controllerLokiConfigService: controllerLokiConfigService,
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

// GetControllerLokiConfig reports the controller-wide Loki configuration for
// the agent specified.
func (api *LoggerAPIV2) GetControllerLokiConfig(ctx context.Context, arg params.Entity) params.LokiConfigResult {
	tag, err := names.ParseTag(arg.Tag)
	if err != nil {
		return params.LokiConfigResult{Error: apiservererrors.ServerError(err)}
	}
	if !api.authorizer.AuthOwner(tag) {
		return params.LokiConfigResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}
	}

	config, err := api.controllerLokiConfigService.GetLokiConfig(ctx)
	if internalerrors.Is(err, loggingerrors.LokiConfigNotFound) {
		return params.LokiConfigResult{
			Error: apiservererrors.ParamsErrorf(params.CodeNotFound, "loki config not found"),
		}
	}
	if err != nil {
		return params.LokiConfigResult{Error: apiservererrors.ServerError(err)}
	}

	result := params.LokiConfigResult{
		Endpoint:           config.Endpoint,
		InsecureSkipVerify: config.InsecureSkipVerify,
	}
	if config.CACertificate != "" {
		result.CACert = &config.CACertificate
	}
	return result
}

// WatchControllerLokiConfig starts a watcher to track changes to the
// controller-wide Loki configuration for the agent specified.
func (api *LoggerAPIV2) WatchControllerLokiConfig(ctx context.Context, arg params.Entity) params.NotifyWatchResult {
	tag, err := names.ParseTag(arg.Tag)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}
	}
	if !api.authorizer.AuthOwner(tag) {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}
	}

	watch, err := api.controllerLokiConfigService.WatchLokiConfig(ctx)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}
	}

	result := params.NotifyWatchResult{}
	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, watch)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
	}
	return result
}
