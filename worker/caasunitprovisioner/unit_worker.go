// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/worker/catacomb"
)

type unitWorker struct {
	catacomb            catacomb.Catacomb
	application         string
	unit                string
	broker              ContainerBroker
	containerSpecGetter ContainerSpecGetter
	lifeGetter          LifeGetter
}

func newUnitWorker(
	application string,
	unit string,
	broker ContainerBroker,
	containerSpecGetter ContainerSpecGetter,
	lifeGetter LifeGetter,
) (worker.Worker, error) {
	w := &unitWorker{
		application:         application,
		unit:                unit,
		broker:              broker,
		containerSpecGetter: containerSpecGetter,
		lifeGetter:          lifeGetter,
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
func (w *unitWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *unitWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *unitWorker) loop() error {
	cw, err := w.containerSpecGetter.WatchContainerSpec(w.unit)
	if err != nil {
		return errors.Trace(err)
	}
	w.catacomb.Add(cw)

	// TODO(caas) -  this loop should also keep an eye on kubernetes and
	// ensure that the unit pod stays up, redeploying it if the pod goes
	// away. For some runtimes we *could* rely on the the runtime's
	// features to do this.

	var currentSpec string
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-cw.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			// If the unit is not alive, don't ask the CAAS broker to create it.
			unitLife, err := w.lifeGetter.Life(w.unit)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			if err != nil || unitLife != life.Alive {
				continue
			}
			specStr, err := w.containerSpecGetter.ContainerSpec(w.unit)
			if errors.IsNotFound(err) {
				// No container spec defined for this unit yet;
				// wait for one to be set.
				logger.Debugf("no container spec defined for %v", w.unit)
				continue
			}
			if err != nil {
				return errors.Trace(err)
			}
			if specStr == currentSpec {
				continue
			}
			currentSpec = specStr

			spec, err := caas.ParseContainerSpec(specStr)
			if err != nil {
				return errors.Annotate(err, "cannot parse container spec")
			}
			if err := w.broker.EnsureUnit(w.application, w.unit, spec); err != nil {
				return errors.Trace(err)
			}
			logger.Debugf("created/updated unit %s", w.unit)
		}
	}
}
