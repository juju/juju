// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/names"
	"github.com/juju/utils/fslock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/uniter/filter"
)

// ManifoldConfig specifies the names a uniter manifold should use to address its
// dependencies.
type ManifoldConfig struct {

	// AgentName must contain the name of a manifold which outputs agent config.
	AgentName string

	// ApiCallerName must contain the name of a manifold which outputs an api
	// connection for the appropriate unit.
	ApiCallerName string

	// EventFilterName must contain the name of a manifold which outputs an event
	// filter for the appropriate unit.
	EventFilterName string

	// LeadershipTrackerName must contain the name of a manifold which outputs a
	// leadership.Tracker for the appropriate unit.
	LeadershipTrackerName string

	// MachineLockName must contain the name of a manifold which outputs the
	// *fslock.Lock which prevents sensitive operations from running at the
	// same time on the same machine.
	MachineLockName string
}

// Manifold returns a dependency.Manifold that governs the construction of a
// Uniter from the named resources defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	// TODO(fwereade): extract the RunListener bits from uniter and make them
	// a real worker with a dependency on the uniter; and have the uniter expose
	// a CommandRunner via an Output func. For now, it can be a leaf node.
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ApiCallerName,
			config.EventFilterName,
			config.LeadershipTrackerName,
			config.MachineLockName,
		},
		Start: startFunc(config),
	}
}

// startFunc returns a StartFunc that creates a worker based on the manifolds
// named in the supplied config.
func startFunc(config ManifoldConfig) dependency.StartFunc {
	return func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
		// Ensure absolute dependencies.
		var leadershipTracker leadership.Tracker
		if err := getResource(config.LeadershipTrackerName, &leadershipTracker); err != nil {
			return nil, err
		}
		var machineLock *fslock.Lock
		if err := getResource(config.MachineLockName, &machineLock); err != nil {
			return nil, err
		}
		var agent agent.Agent
		if err := getResource(config.AgentName, &agent); err != nil {
			return nil, err
		}
		unitTag, ok := agent.Tag().(names.UnitTag)
		if !ok {
			return nil, fmt.Errorf("expected a unit tag; got %q", agent.Tag())
		}

		// This block of dependencies shouldn't really be *required* (we have
		// responsibilities we can and should fulfil even without an api conn),
		// but we don't yet have uniter code paths that can tolerate their
		// absence... so we continue to pass ErrMissing through unhandled.
		var eventFilter filter.Filter
		if err := getResource(config.EventFilterName, &eventFilter); err != nil {
			return nil, err
		}
		var apiCaller base.APICaller
		if err := getResource(config.ApiCallerName, &apiCaller); err != nil {
			return nil, err
		}
		uniterFacade := uniter.NewState(apiCaller, unitTag)
		return NewUniter(
			uniterFacade,
			unitTag,
			leadershipTracker,
			eventFilter,
			agent.CurrentConfig().DataDir(),
			machineLock,
		), nil
	}
}
