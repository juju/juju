// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package restorewatcher

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/state"
)

// Config holds the worker configuration.
type Config struct {
	RestoreInfoWatcher RestoreInfoWatcher
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.RestoreInfoWatcher == nil {
		return errors.NotValidf("nil RestoreInfoWatcher")
	}
	return nil
}

// RestoreInfoWatcher is an interface for watching and obtaining the
// restore info/status.
type RestoreInfoWatcher interface {
	WatchRestoreInfoChanges() state.NotifyWatcher
	RestoreStatus() (state.RestoreStatus, error)
}

// NewWorker returns a new worker that watches for changes to restore
// info, and reports the status to the provided function.
func NewWorker(config Config) (RestoreStatusWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	restoreStatus, err := config.RestoreInfoWatcher.RestoreStatus()
	if err != nil {
		return nil, errors.Trace(err)
	}
	w := &restoreWorker{
		config: config,
		status: restoreStatus,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type restoreWorker struct {
	catacomb catacomb.Catacomb
	config   Config

	mu     sync.Mutex
	status state.RestoreStatus
}

func (w *restoreWorker) loop() error {
	rw := w.config.RestoreInfoWatcher.WatchRestoreInfoChanges()
	w.catacomb.Add(rw)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-rw.Changes():
			status, err := w.config.RestoreInfoWatcher.RestoreStatus()
			if err != nil {
				return errors.Trace(err)
			}
			w.mu.Lock()
			w.status = status
			w.mu.Unlock()
		}
	}
}

// RestoreStatus returns the most recently observed restore status.
func (w *restoreWorker) RestoreStatus() state.RestoreStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

// Kill is part of the worker.Worker interface.
func (w *restoreWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *restoreWorker) Wait() error {
	return w.catacomb.Wait()
}
