// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/common/reboot"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/resolver"
)

// Logger represents the methods used for logging messages.
type Logger interface {
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})

	// Just using the loggo Logger here rather than providing
	// a shim to a Logger. Perhaps we should just have the manifold
	// config take a loggo.Logger?
	Child(string) loggo.Logger
}

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName             string
	APICallerName         string
	MachineLock           machinelock.Lock
	Clock                 clock.Clock
	LeadershipTrackerName string
	CharmDirName          string
	HookRetryStrategyName string
	TranslateResolverErr  func(error) error
	Logger                Logger
}

// Manifold returns a dependency manifold that runs a uniter worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.LeadershipTrackerName,
			config.CharmDirName,
			config.HookRetryStrategyName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if config.Clock == nil {
				return nil, errors.NotValidf("missing Clock")
			}
			if config.MachineLock == nil {
				return nil, errors.NotValidf("missing MachineLock")
			}

			// Collect all required resources.
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiConn api.Connection
			if err := context.Get(config.APICallerName, &apiConn); err != nil {
				// TODO(fwereade): absence of an APICaller shouldn't be the end of
				// the world -- we ought to return a type that can at least run the
				// leader-deposed hook -- but that's not done yet.
				return nil, err
			}
			var leadershipTracker leadership.TrackerWorker
			if err := context.Get(config.LeadershipTrackerName, &leadershipTracker); err != nil {
				return nil, err
			}
			var charmDirGuard fortress.Guard
			if err := context.Get(config.CharmDirName, &charmDirGuard); err != nil {
				return nil, err
			}

			var hookRetryStrategy params.RetryStrategy
			if err := context.Get(config.HookRetryStrategyName, &hookRetryStrategy); err != nil {
				return nil, err
			}

			downloader := api.NewCharmDownloader(apiConn)

			manifoldConfig := config
			// Configure and start the uniter.
			agentConfig := agent.CurrentConfig()
			tag := agentConfig.Tag()
			unitTag, ok := tag.(names.UnitTag)
			if !ok {
				return nil, errors.Errorf("expected a unit tag, got %v", tag)
			}
			uniterFacade := uniter.NewState(apiConn, unitTag)
			uniter, err := NewUniter(&UniterParams{
				UniterFacade:         uniterFacade,
				UnitTag:              unitTag,
				ModelType:            model.IAAS,
				LeadershipTracker:    leadershipTracker,
				DataDir:              agentConfig.DataDir(),
				Downloader:           downloader,
				MachineLock:          manifoldConfig.MachineLock,
				CharmDirGuard:        charmDirGuard,
				UpdateStatusSignal:   NewUpdateStatusTimer(),
				HookRetryStrategy:    hookRetryStrategy,
				NewOperationExecutor: operation.NewExecutor,
				TranslateResolverErr: config.TranslateResolverErr,
				Clock:                manifoldConfig.Clock,
				RebootQuerier:        reboot.NewMonitor(agentConfig.TransientDataDir()),
				Logger:               config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return uniter, nil
		},
	}
}

// TranslateFortressErrors turns errors returned by dependent
// manifolds due to fortress lockdown (i.e. model migration) into an
// error which causes the resolver loop to be restarted. When this
// happens the uniter is about to be shut down anyway.
func TranslateFortressErrors(err error) error {
	if fortress.IsFortressError(err) {
		return resolver.ErrRestart
	}
	return err
}
