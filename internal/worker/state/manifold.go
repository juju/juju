// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdcontext "context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"github.com/juju/worker/v3/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.worker.state")

// ManifoldConfig provides the dependencies for Manifold.
type ManifoldConfig struct {
	AgentName              string
	StateConfigWatcherName string
	OpenStatePool          func(stdcontext.Context, coreagent.Config) (*state.StatePool, error)
	PingInterval           time.Duration

	// SetStatePool is called with the state pool when it is created,
	// and called again with nil just before the state pool is closed.
	// This is used for publishing the state pool to the agent's
	// introspection worker, which runs outside of the dependency
	// engine; hence the manifold's Output cannot be relied upon.
	SetStatePool func(*state.StatePool)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.StateConfigWatcherName == "" {
		return errors.NotValidf("empty StateConfigWatcherName")
	}
	if config.OpenStatePool == nil {
		return errors.NotValidf("nil OpenStatePool")
	}
	if config.SetStatePool == nil {
		return errors.NotValidf("nil SetStatePool")
	}
	return nil
}

const defaultPingInterval = 15 * time.Second

// Manifold returns a manifold whose worker which wraps a
// *state.State, which is in turn wrapper by a StateTracker.  It will
// exit if the State's associated mongodb session dies.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.StateConfigWatcherName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			// Get the agent.
			var agent coreagent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}

			// Confirm we're running in a state server by asking the
			// stateconfigwatcher manifold.
			var haveStateConfig bool
			if err := context.Get(config.StateConfigWatcherName, &haveStateConfig); err != nil {
				return nil, err
			}
			if !haveStateConfig {
				return nil, errors.Annotate(dependency.ErrMissing, "no StateServingInfo in config")
			}

			pool, err := config.OpenStatePool(stdcontext.Background(), agent.CurrentConfig())
			if err != nil {
				return nil, errors.Trace(err)
			}
			stTracker := newStateTracker(pool)

			pingInterval := config.PingInterval
			if pingInterval == 0 {
				pingInterval = defaultPingInterval
			}

			w := &stateWorker{
				stTracker:    stTracker,
				pingInterval: pingInterval,
				setStatePool: config.SetStatePool,
			}
			if err := catacomb.Invoke(catacomb.Plan{
				Site: &w.catacomb,
				Work: w.loop,
			}); err != nil {
				if err := stTracker.Done(); err != nil {
					logger.Warningf("error releasing state: %v", err)
				}
				return nil, errors.Trace(err)
			}
			return w, nil
		},
		Output: outputFunc,
	}
}

// outputFunc extracts a *StateTracker from a *stateWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*stateWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *StateTracker:
		*outPointer = inWorker.stTracker
	default:
		return errors.Errorf("out should be *StateTracker; got %T", out)
	}
	return nil
}
