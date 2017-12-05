// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/worker/catacomb"
)

type unitWorker struct {
	catacomb            catacomb.Catacomb
	unit                string
	broker              ContainerBroker
	containerSpecGetter ContainerSpecGetter
}

func newUnitWorker(
	unit string,
	broker ContainerBroker,
	containerSpecGetter ContainerSpecGetter,
) (worker.Worker, error) {
	w := &unitWorker{
		unit:                unit,
		broker:              broker,
		containerSpecGetter: containerSpecGetter,
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
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-cw.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			spec, err := w.containerSpecGetter.ContainerSpec(w.unit)
			if errors.IsNotFound(err) {
				// No container spec defined for this unit yet;
				// wait for one to be set.
				continue
			}
			if err != nil {
				return errors.Trace(err)
			}
			if err := w.broker.EnsureUnit(w.unit, spec); err != nil {
				return errors.Trace(err)
			}
			logger.Debugf("created/updated unit %s", w.unit)
		}
	}
}
