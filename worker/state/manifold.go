// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"
	"github.com/juju/worker/v2/dependency"
	"gopkg.in/tomb.v2"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	"github.com/juju/juju/wrench"
)

var logger = loggo.GetLogger("juju.worker.state")

// ManifoldConfig provides the dependencies for Manifold.
type ManifoldConfig struct {
	AgentName              string
	StateConfigWatcherName string
	OpenStatePool          func(coreagent.Config) (*state.StatePool, error)
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

			pool, err := config.OpenStatePool(agent.CurrentConfig())
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

type stateWorker struct {
	catacomb     catacomb.Catacomb
	stTracker    StateTracker
	pingInterval time.Duration
	setStatePool func(*state.StatePool)
	cleanupOnce  sync.Once
}

func (w *stateWorker) loop() error {
	pool, err := w.stTracker.Use()
	if err != nil {
		return errors.Trace(err)
	}
	defer w.stTracker.Done()

	systemState := pool.SystemState()

	w.setStatePool(pool)
	defer w.setStatePool(nil)

	modelWatcher := systemState.WatchModelLives()
	w.catacomb.Add(modelWatcher)

	modelStateWorkers := make(map[string]worker.Worker)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case modelUUIDs := <-modelWatcher.Changes():
			for _, modelUUID := range modelUUIDs {
				if err := w.processModelLifeChange(
					modelUUID,
					modelStateWorkers,
					pool,
				); err != nil {
					return errors.Trace(err)
				}
			}
		// Useful for tracking down some bugs that occur when
		// mongo is overloaded.
		case <-time.After(30 * time.Second):
			if wrench.IsActive("state-worker", "io-timeout") {
				return errors.Errorf("wrench simulating i/o timeout!")
			}
		}
	}
}

// Report conforms to the Dependency Engine Report() interface, giving an opportunity to introspect
// what is going on at runtime.
func (w *stateWorker) Report() map[string]interface{} {
	return w.stTracker.Report()
}

func (w *stateWorker) processModelLifeChange(
	modelUUID string,
	modelStateWorkers map[string]worker.Worker,
	pool *state.StatePool,
) error {
	remove := func() {
		if w, ok := modelStateWorkers[modelUUID]; ok {
			w.Kill()
			delete(modelStateWorkers, modelUUID)
		}
		pool.Remove(modelUUID)
	}

	model, hp, err := pool.GetModel(modelUUID)
	if err != nil {
		if errors.IsNotFound(err) {
			// Model has been removed from state.
			logger.Debugf("model %q removed from state", modelUUID)
			remove()
			return nil
		}
		return errors.Trace(err)
	}
	defer hp.Release()

	if model.Life() == state.Dead {
		// Model is Dead, and will soon be removed from state.
		logger.Debugf("model %q is dead", modelUUID)
		remove()
		return nil
	}

	if modelStateWorkers[modelUUID] == nil {
		mw := newModelStateWorker(pool, modelUUID, w.pingInterval)
		modelStateWorkers[modelUUID] = mw
		w.catacomb.Add(mw)
	}

	return nil
}

// Kill is part of the worker.Worker interface.
func (w *stateWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *stateWorker) Wait() error {
	err := w.catacomb.Wait()
	w.cleanupOnce.Do(func() {
		// Make sure the worker has exited before closing state.
		if err := w.stTracker.Done(); err != nil {
			logger.Warningf("error releasing state: %v", err)
		}
	})
	return err
}

type modelStateWorker struct {
	tomb         tomb.Tomb
	pool         *state.StatePool
	modelUUID    string
	pingInterval time.Duration
}

func newModelStateWorker(
	pool *state.StatePool,
	modelUUID string,
	pingInterval time.Duration,
) worker.Worker {
	w := &modelStateWorker{
		pool:         pool,
		modelUUID:    modelUUID,
		pingInterval: pingInterval,
	}
	w.tomb.Go(w.loop)
	return w
}

func (w *modelStateWorker) loop() error {
	st, err := w.pool.Get(w.modelUUID)
	if err != nil {
		if errors.IsNotFound(err) {
			// ignore not found error here, because the pooledState has already been removed.
			return nil
		}
		return errors.Trace(err)
	}
	defer func() {
		st.Release()
		w.pool.Remove(w.modelUUID)
	}()

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
func (w *modelStateWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *modelStateWorker) Wait() error {
	return w.tomb.Wait()
}
