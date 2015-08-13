// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/fslock"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/uniter/operation"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName             string
	APICallerName         string
	MachineLockName       string
	LeadershipTrackerName string
}

// Manifold returns a dependency manifold that runs a uniter worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.LeadershipTrackerName,
			config.MachineLockName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {

			// Collect all required resources.
			var agent agent.Agent
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiCaller base.APICaller
			if err := getResource(config.APICallerName, &apiCaller); err != nil {
				// TODO(fwereade): absence of an APICaller shouldn't be the end of
				// the world -- we ought to return a type that can at least run the
				// leader-deposed hook -- but that's not done yet.
				return nil, err
			}
			var machineLock *fslock.Lock
			if err := getResource(config.MachineLockName, &machineLock); err != nil {
				return nil, err
			}
			var leadershipTracker leadership.Tracker
			if err := getResource(config.LeadershipTrackerName, &leadershipTracker); err != nil {
				return nil, err
			}

			// Configure and start the uniter.
			config := agent.CurrentConfig()
			tag := config.Tag()
			unitTag, ok := tag.(names.UnitTag)
			if !ok {
				return nil, errors.Errorf("expected a unit tag, got %v", tag)
			}
			uniterFacade := uniter.NewState(apiCaller, unitTag)
			return NewUniter(&UniterParams{
				UniterFacade:         uniterFacade,
				UnitTag:              unitTag,
				LeadershipTracker:    leadershipTracker,
				DataDir:              config.DataDir(),
				MachineLock:          machineLock,
				MetricsTimerChooser:  NewMetricsTimerChooser(),
				UpdateStatusSignal:   NewUpdateStatusTimer(),
				NewOperationExecutor: operation.NewExecutor,
			}), nil
		},
	}
}
