// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstorepruner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	coreobjectstore "github.com/juju/juju/core/objectstore"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

// TrackedObjectStore is a ObjectStore that is also a worker, to ensure the
// lifecycle of the objectStore is managed.
type TrackedObjectStore interface {
	worker.Worker
	coreobjectstore.ObjectStore
}

// WorkerConfig encapsulates the configuration options for the
// objectStore worker.
type WorkerConfig struct {
	Clock  clock.Clock
	Logger Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

type objectStorePrunerWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	tomb           tomb.Tomb
}

// NewWorker creates a new object store worker.
func NewWorker(cfg WorkerConfig) (*objectStorePrunerWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*objectStorePrunerWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &objectStorePrunerWorker{
		internalStates: internalStates,
		cfg:            cfg,
	}

	w.tomb.Go(w.loop)

	return w, nil
}

func (w *objectStorePrunerWorker) loop() (err error) {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	_ = ctx

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *objectStorePrunerWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *objectStorePrunerWorker) Wait() error {
	return w.tomb.Wait()
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *objectStorePrunerWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}

func (w *objectStorePrunerWorker) reportInternalState(state string) {
	select {
	case <-w.tomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
