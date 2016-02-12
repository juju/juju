// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package hookretrystrategy

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/names"
)

// Right now, these are defined as constants, but the plan is to maybe make
// them configurable in the future
const (
	MinRetryTime    = 5 * time.Second
	MaxRetryTime    = 5 * time.Minute
	JitterRetryTime = true
	RetryTimeFactor = 2
)

func init() {
	common.RegisterStandardFacade("HookRetryStrategy", 1, NewHookRetryStrategyAPI)
}

// HookRetryStrategy defined the methods exported by the hookretrystrategy API facade.
type HookRetryStrategy interface {
	HookRetryStrategy(params.Entities) (params.HookRetryStrategyResults, error)
	WatchHookRetryStrategy(params.Entities) (params.NotifyWatchResults, error)
}

// HookRetryStrategyAPI implements HookRetryStrategy
type HookRetryStrategyAPI struct {
	st         *state.State
	accessUnit common.GetAuthFunc
	resources  *common.Resources
}

var _ HookRetryStrategy = (*HookRetryStrategyAPI)(nil)

// NewHookRetryStrategyAPI creates a new API endpoint for getting hook retry strategies.
func NewHookRetryStrategyAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*HookRetryStrategyAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	return &HookRetryStrategyAPI{
		st: st,
		accessUnit: func() (common.AuthFunc, error) {
			return authorizer.AuthOwner, nil
		},
		resources: resources,
	}, nil
}

// HookRetryStrategy returns HookRetryStrategyResults that can be used by the uniter to configure
// how hooks get retried.
func (h *HookRetryStrategyAPI) HookRetryStrategy(args params.Entities) (params.HookRetryStrategyResults, error) {
	results := params.HookRetryStrategyResults{
		Results: make([]params.HookRetryStrategyResult, len(args.Entities)),
	}
	canAccess, err := h.accessUnit()
	if err != nil {
		return params.HookRetryStrategyResults{}, errors.Trace(err)
	}
	config, configErr := h.st.ModelConfig()
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			if configErr == nil {
				// Right now the only real configurable value is ShouldRetry,
				// which is taken from the environment
				// The rest are hardcoded
				results.Results[i].Result = &params.HookRetryStrategy{
					ShouldRetry:     config.AutomaticallyRetryHooks(),
					MinRetryTime:    MinRetryTime,
					MaxRetryTime:    MaxRetryTime,
					JitterRetryTime: JitterRetryTime,
					RetryTimeFactor: RetryTimeFactor,
				}
				err = nil
			} else {
				err = configErr
			}
		}
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// WatchHookRetryStrategy watches for changes to the environment. Currently we only allow
// changes to the boolean that determines whether hooks should be retried or not.
func (h *HookRetryStrategyAPI) WatchHookRetryStrategy(args params.Entities) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := h.accessUnit()
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
			watch := h.st.WatchForModelConfigChanges()
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
