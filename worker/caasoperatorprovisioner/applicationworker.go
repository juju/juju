// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/worker/catacomb"
)

// applicationWorker listens for changes to caas units
// associated with the application and records these in
// the Juju model.
type applicationWorker struct {
	catacomb catacomb.Catacomb

	applicationName string
	broker          caas.Broker
	facade          CAASProvisionerFacade
}

// Kill is defined on worker.Worker
func (w *applicationWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *applicationWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *applicationWorker) loop() (err error) {
	unitsWatcher, err := w.broker.WatchUnits(w.applicationName)
	if err != nil {
		return errors.Annotatef(err, "failed to start unit watcher for %q", w.applicationName)
	}
	if err := w.catacomb.Add(unitsWatcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-unitsWatcher.Changes():
			logger.Debugf("units changed: %#v", ok)
			if !ok {
				return unitsWatcher.Wait()
			}
			units, err := w.broker.Units(w.applicationName)
			if err != nil {
				return errors.Trace(err)
			}
			logger.Infof("units for %v: %+v", w.applicationName, units)
			// TODO(caas) - record changes in Juju model.
		}
	}
}
