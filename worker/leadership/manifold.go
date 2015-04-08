// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName           string
	ApiConnectionName   string
	LeadershipGuarantee time.Duration
}

// Manifold returns a manifold whose worker wraps a Tracker working on behalf of
// the dependency identified by AgentName.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ApiConnectionName,
		},
		Start:  startFunc(config),
		Output: outputFunc,
	}
}

// startFunc returns a StartFunc that creates a worker based on the manifolds
// named in the supplied config.
func startFunc(config ManifoldConfig) dependency.StartFunc {
	return func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
		var agent agent.Agent
		if err := getResource(config.AgentName, &agent); err != nil {
			return nil, err
		}
		unitTag, ok := agent.Tag().(names.UnitTag)
		if !ok {
			return nil, fmt.Errorf("expected a unit tag; got %q", agent.Tag())
		}
		var apiConnection *api.State
		if err := getResource(config.ApiConnectionName, &apiConnection); err != nil {
			return nil, err
		}
		return NewTrackerWorker(
			unitTag,
			apiConnection.LeadershipManager(),
			config.LeadershipGuarantee,
		), nil
	}
}

// outputFunc extracts the Tracker from a TrackerWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(Tracker)
	outPointer, _ := out.(*Tracker)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker
	return nil
}
