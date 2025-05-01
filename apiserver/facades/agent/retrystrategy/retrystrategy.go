// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Right now, these are defined as constants, but the plan is to maybe make
// them configurable in the future
const (
	MinRetryTime    = 5 * time.Second
	MaxRetryTime    = 5 * time.Minute
	JitterRetryTime = true
	RetryTimeFactor = 2
)

// RetryStrategy defines the methods exported by the RetryStrategy API facade.
type RetryStrategy interface {
	RetryStrategy(context.Context, params.Entities) (params.RetryStrategyResults, error)
	WatchRetryStrategy(context.Context, params.Entities) (params.NotifyWatchResults, error)
}

// RetryStrategyAPI implements RetryStrategy
type RetryStrategyAPI struct {
	canAccess          common.GetAuthFunc
	modelConfigService ModelConfigService
	watcherRegistry    facade.WatcherRegistry
}

var _ RetryStrategy = (*RetryStrategyAPI)(nil)

func NewRetryStrategyAPI(
	authorizer facade.Authorizer,
	modelConfigService ModelConfigService,
	watcherRegistry facade.WatcherRegistry,
) (*RetryStrategyAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &RetryStrategyAPI{
		canAccess: func(context.Context) (common.AuthFunc, error) {
			return authorizer.AuthOwner, nil
		},
		modelConfigService: modelConfigService,
		watcherRegistry:    watcherRegistry,
	}, nil
}

// RetryStrategy returns RetryStrategyResults that can be used by any code that uses
// to configure the retry timer that's currently in juju utils.
func (h *RetryStrategyAPI) RetryStrategy(ctx context.Context, args params.Entities) (params.RetryStrategyResults, error) {
	results := params.RetryStrategyResults{
		Results: make([]params.RetryStrategyResult, len(args.Entities)),
	}
	canAccess, err := h.canAccess(ctx)
	if err != nil {
		return params.RetryStrategyResults{}, errors.Trace(err)
	}
	config, err := h.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return params.RetryStrategyResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			// Right now the only real configurable value is ShouldRetry,
			// which is taken from the model.
			// The rest are hardcoded.
			results.Results[i].Result = &params.RetryStrategy{
				ShouldRetry:     config.AutomaticallyRetryHooks(),
				MinRetryTime:    MinRetryTime,
				MaxRetryTime:    MaxRetryTime,
				JitterRetryTime: JitterRetryTime,
				RetryTimeFactor: RetryTimeFactor,
			}
			err = nil
		}
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// WatchRetryStrategy watches for changes to the model. Currently we only allow
// changes to the boolean that determines whether retries should be attempted or not.
func (h *RetryStrategyAPI) WatchRetryStrategy(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := h.canAccess(ctx)
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canAccess(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		watch, err := h.modelConfigService.Watch()
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		notifyWatcher, err := watcher.Normalise[[]string](watch)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results.Results[i].NotifyWatcherId, _, err = internal.EnsureRegisterWatcher[struct{}](ctx, h.watcherRegistry, notifyWatcher)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
}
