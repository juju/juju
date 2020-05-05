// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

// TODO(menn0) - note that this is currently unused, pending further
// refactoring of state.State and state.Controller.

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"gopkg.in/tomb.v2"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.worker.controller")

// ManifoldConfig provides the dependencies for Manifold.
type ManifoldConfig struct {
	AgentName              string
	StateConfigWatcherName string
	OpenController         func(coreagent.Config) (*state.Controller, error)
	MongoPingInterval      time.Duration
}

const defaultMongoPingInterval = 15 * time.Second

// Manifold returns a manifold whose worker which wraps a
// *state.State, which is in turn wrapped by a Tracker.  It will
// exit if the State's associated mongodb session dies.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.StateConfigWatcherName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			// First, a sanity check.
			if config.OpenController == nil {
				return nil, errors.New("OpenController is nil in config")
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

			ctlr, err := config.OpenController(agent.CurrentConfig())
			if err != nil {
				return nil, errors.Trace(err)
			}
			tracker := newTracker(ctlr)

			mongoPingInterval := config.MongoPingInterval
			if mongoPingInterval == 0 {
				mongoPingInterval = defaultMongoPingInterval
			}

			w := &controllerWorker{
				tracker:           tracker,
				mongoPingInterval: mongoPingInterval,
			}
			w.tomb.Go(func() error {
				loopErr := w.loop()
				if err := tracker.Done(); err != nil {
					logger.Errorf("error releasing state: %v", err)
				}
				return loopErr
			})
			return w, nil
		},
		Output: outputFunc,
	}
}

// outputFunc extracts a *Tracker from a Worker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*controllerWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *Tracker:
		*outPointer = inWorker.tracker
	default:
		return errors.Errorf("out should be *Tracker; got %T", out)
	}
	return nil
}

type controllerWorker struct {
	tomb              tomb.Tomb
	tracker           Tracker
	mongoPingInterval time.Duration
}

func (w *controllerWorker) loop() error {
	ctlr, err := w.tracker.Use()
	if err != nil {
		return errors.Annotate(err, "failed to obtain controller")
	}
	defer w.tracker.Done()

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(w.mongoPingInterval):
			if err := ctlr.Ping(); err != nil {
				return errors.Annotate(err, "database ping failed")
			}
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *controllerWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *controllerWorker) Wait() error {
	return w.tomb.Wait()
}
