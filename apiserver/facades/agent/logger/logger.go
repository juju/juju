// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/environs/config"
)

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
	controller *cache.Controller
	model      *cache.Model
	resources  facade.Resources
	authorizer facade.Authorizer
}

var _ Logger = (*LoggerAPI)(nil)

// NewLoggerAPI creates a new server-side logger API end point.
func NewLoggerAPI(ctx facade.Context) (*LoggerAPI, error) {
	st := ctx.State()
	resources := ctx.Resources()
	authorizer := ctx.Auth()

	if !authorizer.AuthMachineAgent() && !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, common.ErrPerm
	}
	m, err := ctx.Controller().Model(st.ModelUUID())
	if err != nil {
		return nil, err
	}

	return &LoggerAPI{
		controller: ctx.Controller(),
		model:      m,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// WatchLoggingConfig starts a watcher to track changes to the logging config
// for the agents specified..  Unfortunately the current infrastructure makes
// watching parts of the config non-trivial, so currently any change to the
// config will cause the watcher to notify the client.
func (api *LoggerAPI) WatchLoggingConfig(arg params.Entities) params.NotifyWatchResults {
	result := make([]params.NotifyWatchResult, len(arg.Entities))
	for i, entity := range arg.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		err = common.ErrPerm
		if api.authorizer.AuthOwner(tag) {
			watch := api.model.WatchConfig("logging-config")
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
		result[i].Error = common.ServerError(err)
	}
	return params.NotifyWatchResults{Results: result}
}

// LoggingConfig reports the logging configuration for the agents specified.
func (api *LoggerAPI) LoggingConfig(arg params.Entities) params.StringResults {
	if len(arg.Entities) == 0 {
		return params.StringResults{}
	}
	results := make([]params.StringResult, len(arg.Entities))
	// TODO: ensure that the cache model can return a proper config object.
	config, configErr := config.New(config.NoDefaults, api.model.Config())
	for i, entity := range arg.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		err = common.ErrPerm
		if api.authorizer.AuthOwner(tag) {
			if configErr == nil {
				results[i].Result = config.LoggingConfig()
				err = nil
			} else {
				err = configErr
			}
		}
		results[i].Error = common.ServerError(err)
	}
	return params.StringResults{Results: results}
}
