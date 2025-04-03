// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/wrench"
	"github.com/juju/juju/state"
)

type stateWorker struct {
	catacomb     catacomb.Catacomb
	stTracker    StateTracker
	pingInterval time.Duration
	setStatePool func(*state.StatePool)
	cleanupOnce  sync.Once

	pool              *state.StatePool
	modelStateWorkers map[string]worker.Worker
}

func (w *stateWorker) loop() error {
	pool, systemState, err := w.stTracker.Use()
	if err != nil {
		return errors.Trace(err)
	}
	w.pool = pool
	defer func() { _ = w.stTracker.Done() }()

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

// Report conforms to the Dependency Engine Report() interface, giving an opportunity to introspect
// what is going on at runtime.
func (w *stateWorker) Report() map[string]interface{} {
	return w.stTracker.Report()
}

func (w *stateWorker) processModelLifeChange(modelUUID string) error {
	model, hp, err := w.pool.GetModel(modelUUID)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// Model has been removed from state.
			logger.Debugf(context.Background(), "model %q removed from state", modelUUID)
			w.remove(modelUUID)
			return nil
		}
		return errors.Trace(err)
	}
	defer hp.Release()

	if model.Life() == state.Dead {
		// Model is Dead, and will soon be removed from state.
		logger.Debugf(context.Background(), "model %q is dead", modelUUID)
		w.remove(modelUUID)

		_, _ = w.pool.Remove(modelUUID)

		return nil
	}

	if w.modelStateWorkers[modelUUID] == nil {
		mw := newModelStateWorker(w.pool, modelUUID, w.pingInterval)
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
			logger.Warningf(context.Background(), "error releasing state: %v", err)
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
		if errors.Is(err, errors.NotFound) {
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

	// Jitter the interval so that each model doesn't attempt to connect to
	// mongo at potentially the same time.
	interval := w.pingInterval + jitter(time.Millisecond*200)

	// If the state ping fails, attempt to retry the ping, before returning.
	var pingErr error
	for attempt := 0; attempt < 5; {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(interval):
			if pingErr = st.Ping(); pingErr != nil {
				// Reduce the next ping interval to fail early in case mongo
				// has actually died. This should prevent the worst case
				// scenario of a large initial ping interval.
				interval = maxDuration(interval/2, time.Second)
				attempt++
				continue
			}
			interval = w.pingInterval
			attempt = 0
		}
	}

	return errors.Annotate(pingErr, "state ping failed")
}

// Kill is part of the worker.Worker interface.
func (w *modelStateWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *modelStateWorker) Wait() error {
	return w.tomb.Wait()
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func jitter(amount time.Duration) time.Duration {
	return time.Duration((rand.Float64() - 0.5) * float64(amount))
}
