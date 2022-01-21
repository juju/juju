// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statemanager

import (
	"context"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/overlord"
	"github.com/juju/juju/overlord/state"
)

// Overlord defines the various managers that are available for the whole
// state.
type Overlord interface {
	StartUp(context.Context) error
	Stop() error
	State() overlord.State

	LogManager() overlord.LogManager
}

// WorkerConfig encapsulates the configuration options for the
// statemanager worker.
type WorkerConfig struct {
	DBAccessor DBAccessor
	Logger     Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.DBAccessor == nil {
		return errors.NotValidf("missing DBAccessor")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

type stateManagerWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	mutex    sync.Mutex
	managers map[string]Overlord
}

// NewWorker creates a new state manager worker.
func NewWorker(cfg WorkerConfig) (*stateManagerWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &stateManagerWorker{
		cfg:      cfg,
		managers: make(map[string]Overlord),
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *stateManagerWorker) loop() (err error) {
	defer func() {
		w.mutex.Lock()
		defer w.mutex.Unlock()

		for ns, mgr := range w.managers {
			if mErr := mgr.Stop(); mErr != nil {
				w.cfg.Logger.Errorf("failed to stop manager %q with error %v", ns, mErr)
			}
		}
	}()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *stateManagerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *stateManagerWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *stateManagerWorker) GetStateManager(namespace string) (Overlord, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if mgr, ok := w.managers[namespace]; ok && mgr != nil {
		return mgr, errors.Annotatef(mgr.StartUp(ctx), "state manager startup failure for %q", namespace)
	}

	db, err := w.cfg.DBAccessor.GetDB(namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

	st := state.NewState(db)

	var mgr Overlord
	switch namespace {
	case "logs":
		mgr, err = overlord.NewLogOverlord(st)
	default:
		mgr, err = overlord.NewModelOverlord(st)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	w.managers[namespace] = mgr

	return mgr, errors.Annotatef(mgr.StartUp(ctx), "state manager startup failure for %q", namespace)
}
