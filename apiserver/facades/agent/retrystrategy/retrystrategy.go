// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
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
	RetryStrategy(params.Entities) (params.RetryStrategyResults, error)
	WatchRetryStrategy(params.Entities) (params.NotifyWatchResults, error)
}

// RetryStrategyAPI implements RetryStrategy
type RetryStrategyAPI struct {
	st        *state.State
	model     *state.Model
	canAccess common.GetAuthFunc
	resources facade.Resources
}

var _ RetryStrategy = (*RetryStrategyAPI)(nil)

// NewRetryStrategyAPI creates a new API endpoint for getting retry strategies.
func NewRetryStrategyAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*RetryStrategyAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, common.ErrPerm
	}

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &RetryStrategyAPI{
		st:    st,
		model: model,
		canAccess: func() (common.AuthFunc, error) {
			return authorizer.AuthOwner, nil
		},
		resources: resources,
	}, nil
}

// RetryStrategy returns RetryStrategyResults that can be used by any code that uses
// to configure the retry timer that's currently in juju utils.
func (h *RetryStrategyAPI) RetryStrategy(args params.Entities) (params.RetryStrategyResults, error) {
	results := params.RetryStrategyResults{
		Results: make([]params.RetryStrategyResult, len(args.Entities)),
	}
	canAccess, err := h.canAccess()
	if err != nil {
		return params.RetryStrategyResults{}, errors.Trace(err)
	}
	config, err := h.model.ModelConfig()
	if err != nil {
		return params.RetryStrategyResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		err = common.ErrPerm
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
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// WatchRetryStrategy watches for changes to the model. Currently we only allow
// changes to the boolean that determines whether retries should be attempted or not.
func (h *RetryStrategyAPI) WatchRetryStrategy(args params.Entities) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := h.canAccess()
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			watch := h.model.WatchForModelConfigChanges()
			// Consume the initial event. Technically, API calls to Watch
			// 'transmit' the initial event in the Watch response. But
			// NotifyWatchers have no state to transmit.
			if _, ok := <-watch.Changes(); ok {
				results.Results[i].NotifyWatcherId = h.resources.Register(watch)
				err = nil
			} else {
				err = watcher.EnsureErr(watch)
			}
		}
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}
