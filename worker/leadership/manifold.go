// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/leadership"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName           string
	APICallerName       string
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
	return func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
		var agent agent.Agent
		if err := getResource(config.AgentName, &agent); err != nil {
			return nil, err
		}
		var apiCaller base.APICaller
		if err := getResource(config.APICallerName, &apiCaller); err != nil {
			return nil, err
		}
		return newManifoldWorker(agent, apiCaller, config.LeadershipGuarantee)
	}
}

// newManifoldWorker wraps NewTrackerWorker for the convenience of startFunc. It
// exists primarily to be patched out via NewManifoldWorker for ease of testing,
// and is not itself directly tested; once all NewTrackerWorker clients have been
// replaced with manifolds, the tests can be tidied up a bit.
var newManifoldWorker = func(agent agent.Agent, apiCaller base.APICaller, guarantee time.Duration) (worker.Worker, error) {
	tag := agent.CurrentConfig().Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return nil, fmt.Errorf("expected a unit tag; got %q", tag)
	}
	claimer := leadership.NewClient(apiCaller)
	return NewTrackerWorker(unitTag, claimer, guarantee), nil
}

// outputFunc extracts the Tracker from a *tracker passed in as a Worker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*tracker)
	outPointer, _ := out.(*Tracker)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker
	return nil
}
