// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package restorewatcher

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/catacomb"
)

// Config holds the worker configuration.
type Config struct {
	RestoreInfoWatcher   RestoreInfoWatcher
	RestoreStatusChanged RestoreStatusChangedFunc
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.RestoreInfoWatcher == nil {
		return errors.NotValidf("nil RestoreInfoWatcher")
	}
	if config.RestoreStatusChanged == nil {
		return errors.NotValidf("nil RestoreStatusChanged")
	}
	return nil
}

// RestoreInfoWatcher is an interface for watching and obtaining the
// restore info/status.
type RestoreInfoWatcher interface {
	WatchRestoreInfoChanges() state.NotifyWatcher
	RestoreStatus() (state.RestoreStatus, error)
}

// RestoreStatusChangedFunc is the type of a function that will be
// called by the worker whenever the restore status changes.
type RestoreStatusChangedFunc func(state.RestoreStatus) error

// NewWorker returns a new worker that watches for changes to restore
// info, and reports the status to the provided function.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &restoreWorker{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type restoreWorker struct {
	catacomb catacomb.Catacomb
	config   Config
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
			if err := w.config.RestoreStatusChanged(status); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *restoreWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *restoreWorker) Wait() error {
	return w.catacomb.Wait()
}
