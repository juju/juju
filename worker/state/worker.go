// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/wrench"
)

type stateWorker struct {
	catacomb     catacomb.Catacomb
	stTracker    StateTracker
	pingInterval time.Duration
	setStatePool func(*state.StatePool)
	cleanupOnce  sync.Once
	clock        clock.Clock

	pool              *state.StatePool
	modelStateWorkers map[string]worker.Worker
}

func (w *stateWorker) loop() error {
	var err error
	w.pool, err = w.stTracker.Use()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = w.stTracker.Done() }()

	systemState := w.pool.SystemState()

	w.setStatePool(w.pool)
	defer w.setStatePool(nil)

	modelWatcher := systemState.WatchModelLives()
	_ = w.catacomb.Add(modelWatcher)

	wrenchTimer := time.NewTimer(30 * time.Second)
	defer wrenchTimer.Stop()

	w.modelStateWorkers = make(map[string]worker.Worker)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case modelUUIDs := <-modelWatcher.Changes():
			for _, modelUUID := range modelUUIDs {
				if err := w.processModelLifeChange(modelUUID); err != nil {
					return errors.Trace(err)
				}
			}

		case <-time.After(w.pingInterval):

			// Attempt to ping only on the state session, as all model sessions
			// are copies of the parent session.
			err := retry.Call(retry.CallArgs{
				Attempts: 5,
				Func: func() error {
					select {
					case <-w.catacomb.Dying():
						// There is no sentinel error, so wrap it to get the
						// dying error.
						return dyingErr{err: w.catacomb.ErrDying()}
					default:
						return systemState.Ping()
					}
				},
				IsFatalError: func(e error) bool {
					_, isDying := errors.Cause(e).(dyingErr)
					return isDying
				},
				Clock: w.clock,
				Delay: time.Millisecond * 50,
			})
			if err != nil {
				return errors.Annotatef(err, "state ping failed")
			}

		// Useful for tracking down some bugs that occur when
		// mongo is overloaded.
		case <-wrenchTimer.C:
			if wrench.IsActive("state-worker", "io-timeout") {
				return errors.Errorf("wrench simulating i/o timeout!")
			}
			wrenchTimer.Reset(30 * time.Second)
		}
	}
}

// Report conforms to the Dependency Engine Report() interface, giving an
// opportunity to introspect what is going on at runtime.
func (w *stateWorker) Report() map[string]interface{} {
	return w.stTracker.Report()
}

func (w *stateWorker) processModelLifeChange(modelUUID string) error {
	model, hp, err := w.pool.GetModel(modelUUID)
	if err != nil {
		if errors.IsNotFound(err) {
			// Model has been removed from state.
			logger.Debugf("model %q removed from state", modelUUID)
			w.remove(modelUUID)
			return nil
		}
		return errors.Trace(err)
	}
	defer hp.Release()

	if model.Life() == state.Dead {
		// Model is Dead, and will soon be removed from state.
		logger.Debugf("model %q is dead", modelUUID)
		w.remove(modelUUID)
		return nil
	}

	if w.modelStateWorkers[modelUUID] == nil {
		mw := newModelStateWorker(w.pool, modelUUID)
		w.modelStateWorkers[modelUUID] = mw
		_ = w.catacomb.Add(mw)
	}

	return nil
}

func (w *stateWorker) remove(modelUUID string) {
	if worker, ok := w.modelStateWorkers[modelUUID]; ok {
		worker.Kill()
		delete(w.modelStateWorkers, modelUUID)
	}
	_, _ = w.pool.Remove(modelUUID)
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
	tomb      tomb.Tomb
	pool      *state.StatePool
	modelUUID string
}

func newModelStateWorker(
	pool *state.StatePool,
	modelUUID string,
) worker.Worker {
	w := &modelStateWorker{
		pool:      pool,
		modelUUID: modelUUID,
	}
	w.tomb.Go(w.loop)
	return w
}

func (w *modelStateWorker) loop() error {
	st, err := w.pool.Get(w.modelUUID)
	if err != nil {
		if errors.IsNotFound(err) {
			// Ignore not found error here, because the pooledState has already
			// been removed.
			return nil
		}
		return errors.Trace(err)
	}
	defer func() {
		st.Release()
		_, _ = w.pool.Remove(w.modelUUID)
	}()

	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
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

type dyingErr struct {
	err error
}

func (d dyingErr) Error() string {
	return d.err.Error()
}
