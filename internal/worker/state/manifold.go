// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"github.com/juju/worker/v4/dependency"

	coreagent "github.com/juju/juju/agent"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.worker.state")

// ManifoldConfig provides the dependencies for Manifold.
type ManifoldConfig struct {
	AgentName              string
	StateConfigWatcherName string
	DomainServicesName     string
	OpenStatePool          func(context.Context, coreagent.Config, services.ControllerDomainServices, services.DomainServicesGetter) (*state.StatePool, error)
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
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
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
			config.DomainServicesName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			// Get the agent.
			var agent coreagent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}

			// Confirm we're running in a state server by asking the
			// stateconfigwatcher manifold.
			var haveStateConfig bool
			if err := getter.Get(config.StateConfigWatcherName, &haveStateConfig); err != nil {
				return nil, err
			}
			if !haveStateConfig {
				return nil, errors.Annotate(dependency.ErrMissing, "no StateServingInfo in config")
			}

			var controllerDomainServices services.ControllerDomainServices
			if err := getter.Get(config.DomainServicesName, &controllerDomainServices); err != nil {
				return nil, err
			}
			var domainServicesGetter services.DomainServicesGetter
			if err := getter.Get(config.DomainServicesName, &domainServicesGetter); err != nil {
				return nil, err
			}
			pool, err := config.OpenStatePool(context.Background(), agent.CurrentConfig(), controllerDomainServices, domainServicesGetter)
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
				Name: "state",
				Site: &w.catacomb,
				Work: w.loop,
			}); err != nil {
				if err := stTracker.Done(); err != nil {
					logger.Warningf(ctx, "error releasing state: %v", err)
				}
				return nil, errors.Trace(err)
			}
			return w, nil
		},
		Output: outputFunc,
	}
}

// outputFunc extracts a *StateTracker from a *stateWorker.
func outputFunc(in worker.Worker, out any) error {
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
