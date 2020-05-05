// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold
// will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	Clock         clock.Clock
	Interval      time.Duration
	NewFacade     func(base.APICaller) (Facade, error)
	NewWorker     func(Config) (worker.Worker, error)
}

// newWorker is an engine.AgentAPIStartFunc that draws context from the
// ManifoldConfig on which it is defined.
func (config ManifoldConfig) newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {

	// This bit should be encapsulated in another manifold
	// satisfying jujud/agent/engine.Flag, as described in
	// the implementation below. Shouldn't be a concern here.
	if ok, err := apiagent.IsController(apiCaller, a.CurrentConfig().Tag()); err != nil {
		return nil, errors.Trace(err)
	} else if !ok {
		// This depends on a job change triggering an agent
		// bounce, which does happen today, but is not ideal;
		// another reason to use a flag.
		return nil, dependency.ErrMissing
	}

	// Get the API facade.
	if config.NewFacade == nil {
		logger.Errorf("nil NewFacade not valid, uninstalling")
		return nil, dependency.ErrUninstall
	}
	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Start the worker.
	if config.NewWorker == nil {
		logger.Errorf("nil NewWorker not valid, uninstalling")
		return nil, dependency.ErrUninstall
	}
	worker, err := config.NewWorker(Config{
		Facade:   facade,
		Clock:    config.Clock,
		Interval: config.Interval,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold returns a dependency manifold that runs a resumer worker,
// using the resources named or defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	aaConfig := engine.AgentAPIManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
	}
	return engine.AgentAPIManifold(aaConfig, config.newWorker)
}
