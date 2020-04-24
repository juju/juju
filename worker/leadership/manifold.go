// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/leadership"
	coreleadership "github.com/juju/juju/core/leadership"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName           string
	APICallerName       string
	Clock               clock.Clock
	LeadershipGuarantee time.Duration
}

// Manifold returns a manifold whose worker wraps a Tracker working on behalf of
// the dependency identified by AgentName.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start:  startFunc(config),
		Output: outputFunc,
	}
}

// startFunc returns a StartFunc that creates a worker based on the manifolds
// named in the supplied config.
func startFunc(config ManifoldConfig) dependency.StartFunc {
	return func(context dependency.Context) (worker.Worker, error) {
		if config.Clock == nil {
			return nil, errors.NotValidf("missing Clock")
		}
		var agent agent.Agent
		if err := context.Get(config.AgentName, &agent); err != nil {
			return nil, err
		}
		var apiCaller base.APICaller
		if err := context.Get(config.APICallerName, &apiCaller); err != nil {
			return nil, err
		}
		return NewManifoldWorker(agent, apiCaller, config.Clock, config.LeadershipGuarantee)
	}
}

// NewManifoldWorker wraps NewTracker for the convenience of startFunc. It
// exists primarily to be patched out via NewManifoldWorker for ease of testing,
// and is not itself directly tested. It would almost certainly be better to
// pass the constructor dependencies in as explicit manifold config.
var NewManifoldWorker = func(agent agent.Agent, apiCaller base.APICaller, clock clock.Clock, guarantee time.Duration) (worker.Worker, error) {
	tag := agent.CurrentConfig().Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return nil, fmt.Errorf("expected a unit tag; got %q", tag)
	}
	claimer := leadership.NewClient(apiCaller)
	return NewTracker(unitTag, claimer, clock, guarantee), nil
}

// outputFunc extracts the coreleadership.TrackerWorker from a *Tracker passed in as a Worker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*Tracker)
	if inWorker == nil {
		return errors.Errorf("expected *Tracker input; got %T", in)
	}
	outPointer, _ := out.(*coreleadership.TrackerWorker)
	if outPointer == nil {
		return errors.Errorf("expected *leadership.Tracker output; got %T", out)
	}
	*outPointer = inWorker
	return nil
}
