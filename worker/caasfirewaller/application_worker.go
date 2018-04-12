// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/worker/catacomb"
)

type applicationWorker struct {
	catacomb          catacomb.Catacomb
	application       string
	applicationGetter ApplicationGetter
	serviceExposer    ServiceExposer

	lifeGetter LifeGetter

	initial           bool
	previouslyExposed bool
}

func newApplicationWorker(
	application string,
	applicationGetter ApplicationGetter,
	applicationExposer ServiceExposer,
	lifeGetter LifeGetter,
) (worker.Worker, error) {
	w := &applicationWorker{
		application:       application,
		applicationGetter: applicationGetter,
		serviceExposer:    applicationExposer,
		lifeGetter:        lifeGetter,
		initial:           true,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *applicationWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *applicationWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *applicationWorker) loop() (err error) {
	defer func() {
		// If the application has been deleted, we can return nil.
		if errors.IsNotFound(err) {
			logger.Debugf("caas firewaller application %v has been removed", w.application)
			err = nil
		}
	}()
	appWatcher, err := w.applicationGetter.WatchApplication(w.application)
	if err := w.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}
			if err := w.processApplicationChange(); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (w *applicationWorker) processApplicationChange() (err error) {
	exposed, err := w.applicationGetter.IsExposed(w.application)
	if err != nil {
		return errors.Trace(err)
	}
	if !w.initial && exposed == w.previouslyExposed {
		return nil
	}

	w.initial = false
	w.previouslyExposed = exposed
	if exposed {
		appConfig, err := w.applicationGetter.ApplicationConfig(w.application)
		if err != nil {
			return errors.Trace(err)
		}
		if err := w.serviceExposer.ExposeService(w.application, appConfig); err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	if err := w.serviceExposer.UnexposeService(w.application); err != nil {
		return errors.Trace(err)
	}
	return nil
}
