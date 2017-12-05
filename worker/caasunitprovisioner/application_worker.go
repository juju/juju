// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/worker/catacomb"
)

type applicationWorker struct {
	catacomb            catacomb.Catacomb
	application         string
	containerSpecGetter ContainerSpecGetter
	lifeGetter          LifeGetter
	unitGetter          UnitGetter
}

func newApplicationWorker(
	application string,
	containerSpecGetter ContainerSpecGetter,
	lifeGetter LifeGetter,
	unitGetter UnitGetter,
) (worker.Worker, error) {
	w := &applicationWorker{
		application:         application,
		containerSpecGetter: containerSpecGetter,
		lifeGetter:          lifeGetter,
		unitGetter:          unitGetter,
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
	uw, err := aw.unitGetter.WatchUnits(aw.application)
	if err != nil {
		return errors.Trace(err)
	}
	aw.catacomb.Add(uw)

	unitWorkers := make(map[string]worker.Worker)
	for {
		select {
		case <-aw.catacomb.Dying():
			return aw.catacomb.ErrDying()
		case units, ok := <-uw.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			for _, unitId := range units {
				unitLife, err := aw.lifeGetter.Life(unitId)
				if errors.IsNotFound(err) {
					w, ok := unitWorkers[unitId]
					if ok {
						if err := worker.Stop(w); err != nil {
							return errors.Trace(err)
						}
						delete(unitWorkers, unitId)
					}
					continue
				}
				if err != nil {
					return errors.Trace(err)
				}
				if _, ok := unitWorkers[unitId]; ok || unitLife == life.Dead {
					// Already watching the unit. or we're
					// not yet watching it and it's dead.
					continue
				}
				w, err := newUnitWorker(unitId, aw.containerSpecGetter)
				if err != nil {
					return errors.Trace(err)
				}
				unitWorkers[unitId] = w
				aw.catacomb.Add(w)
			}
		}
	}
}
