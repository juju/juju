// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/tomb.v1"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

var logger = loggo.GetLogger("juju.worker.state")

// ManifoldConfig provides the dependencies for Manifold.
type ManifoldConfig struct {
	AgentName              string
	StateConfigWatcherName string
	OpenState              func(coreagent.Config) (*state.State, error)
	PingInterval           time.Duration
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
			// First, a sanity check.
			if config.OpenState == nil {
				return nil, errors.New("OpenState is nil in config")
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
				return nil, dependency.ErrMissing
			}

			st, err := config.OpenState(agent.CurrentConfig())
			if err != nil {
				return nil, errors.Trace(err)
			}
			stTracker := newStateTracker(st)

			pingInterval := config.PingInterval
			if pingInterval == 0 {
				pingInterval = defaultPingInterval
			}

			w := &stateWorker{
				stTracker:    stTracker,
				pingInterval: pingInterval,
			}
			go func() {
				defer w.tomb.Done()
				w.tomb.Kill(w.loop())
				if err := stTracker.Done(); err != nil {
					logger.Errorf("error releasing state: %v", err)
				}
			}()
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
		return errors.Errorf("out should be *state.State; got %T", out)
	}
	return nil
}

type stateWorker struct {
	tomb         tomb.Tomb
	stTracker    StateTracker
	pingInterval time.Duration
}

func (w *stateWorker) loop() error {
	st, err := w.stTracker.Use()
	if err != nil {
		return errors.Annotate(err, "failed to obtain state")
	}
	defer w.stTracker.Done()

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(w.pingInterval):
			if err := st.Ping(); err != nil {
				return errors.Annotate(err, "state ping failed")
			}
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *stateWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *stateWorker) Wait() error {
	return w.tomb.Wait()
}
