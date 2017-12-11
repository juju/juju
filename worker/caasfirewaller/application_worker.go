// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/worker/catacomb"
)

type applicationWorker struct {
	catacomb          catacomb.Catacomb
	application       string
	applicationGetter ApplicationGetter
	serviceExposer    ServiceExposer

	lifeGetter LifeGetter
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

func (aw *applicationWorker) loop() error {
	// TODO(caas) - get this from the application backend
	config := caas.ResourceConfig{}
	config[caas.JujuExternalHostNameKey] = "localhost"

	appWatcher, err := aw.applicationGetter.WatchApplication(aw.application)
	if err != nil {
		if params.IsCodeNotFound(err) {
			return nil
		}
		return errors.Trace(err)
	}
	if err := aw.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}

	var previouslyExposed bool
	initial := true
	for {
		select {
		case <-aw.catacomb.Dying():
			return aw.catacomb.ErrDying()
		case _, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}
			exposed, err := aw.applicationGetter.IsExposed(aw.application)
			if err != nil {
				if !params.IsCodeNotFound(err) {
					return errors.Trace(err)
				}
				return nil
			}
			if !initial && exposed == previouslyExposed {
				continue
			}

			initial = false
			previouslyExposed = exposed
			if exposed {
				if err := aw.serviceExposer.ExposeService(aw.application, config); err != nil {
					return errors.Trace(err)
				}
				continue
			}
			if err := aw.serviceExposer.UnexposeService(aw.application); err != nil {
				return errors.Trace(err)
			}
		}
	}
}
