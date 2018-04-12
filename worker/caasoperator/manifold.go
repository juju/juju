// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apileadership "github.com/juju/juju/api/leadership"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	coreleadership "github.com/juju/juju/core/leadership"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/operation"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	ClockName     string

	MachineLockName       string
	LeadershipGuarantee   time.Duration
	CharmDirName          string
	HookRetryStrategyName string
	TranslateResolverErr  func(error) error

	NewWorker          func(Config) (worker.Worker, error)
	NewClient          func(base.APICaller) Client
	NewCharmDownloader func(base.APICaller) Downloader
}

func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("missing NewWorker")
	}
	if config.NewClient == nil {
		return errors.NotValidf("missing NewClient")
	}
	if config.NewCharmDownloader == nil {
		return errors.NotValidf("missing NewCharmDownloader")
	}
	if config.CharmDirName == "" {
		return errors.NotValidf("missing CharmDirName")
	}
	if config.MachineLockName == "" {
		return errors.NotValidf("missing MachineLockName")
	}
	if config.HookRetryStrategyName == "" {
		return errors.NotValidf("missing HookRetryStrategyName")
	}
	if config.LeadershipGuarantee == 0 {
		return errors.NotValidf("missing LeadershipGuarantee")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a caasoperator worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.ClockName,
			config.CharmDirName,
			config.HookRetryStrategyName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}

			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			client := config.NewClient(apiCaller)
			downloader := config.NewCharmDownloader(apiCaller)

			var clock clock.Clock
			if err := context.Get(config.ClockName, &clock); err != nil {
				return nil, errors.Trace(err)
			}

			modelName, err := client.ModelName()
			if err != nil {
				return nil, errors.Trace(err)
			}

			var charmDirGuard fortress.Guard
			if err := context.Get(config.CharmDirName, &charmDirGuard); err != nil {
				return nil, err
			}

			var hookRetryStrategy params.RetryStrategy
			if err := context.Get(config.HookRetryStrategyName, &hookRetryStrategy); err != nil {
				return nil, err
			}

			// Configure and start the caasoperator worker.
			agentConfig := agent.CurrentConfig()
			tag := agentConfig.Tag()
			applicationTag, ok := tag.(names.ApplicationTag)
			if !ok {
				return nil, errors.Errorf("expected an application tag, got %v", tag)
			}
			newUniterFunc := func(unitTag names.UnitTag) *apiuniter.State {
				return apiuniter.NewState(apiCaller, unitTag)
			}
			leadershipTrackerFunc := func(unitTag names.UnitTag) coreleadership.Tracker {
				claimer := apileadership.NewClient(apiCaller)
				return leadership.NewTracker(unitTag, claimer, clock, config.LeadershipGuarantee)
			}
			w, err := config.NewWorker(Config{
				ModelUUID:          agentConfig.Model().Id(),
				ModelName:          modelName,
				Application:        applicationTag.Id(),
				CharmGetter:        client,
				Clock:              clock,
				PodSpecSetter:      client,
				DataDir:            agentConfig.DataDir(),
				Downloader:         downloader,
				StatusSetter:       client,
				UnitGetter:         client,
				ApplicationWatcher: client,
				StartUniterFunc:    uniter.StartUniter,

				LeadershipTrackerFunc: leadershipTrackerFunc,
				UniterFacadeFunc:      newUniterFunc,
				UniterParams: &uniter.UniterParams{
					NewOperationExecutor: operation.NewExecutor,
					DataDir:              agentConfig.DataDir(),
					Clock:                clock,
					MachineLockName:      config.MachineLockName,
					CharmDirGuard:        charmDirGuard,
					UpdateStatusSignal:   uniter.NewUpdateStatusTimer(),
					HookRetryStrategy:    hookRetryStrategy,
					TranslateResolverErr: config.TranslateResolverErr,
				},
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
