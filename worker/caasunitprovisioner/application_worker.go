// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/worker/catacomb"
)

type applicationWorker struct {
	catacomb           catacomb.Catacomb
	application        string
	brokerManagedUnits bool
	serviceBroker      ServiceBroker
	containerBroker    ContainerBroker

	containerSpecGetter ContainerSpecGetter
	lifeGetter          LifeGetter
	applicationGetter   ApplicationGetter
	unitGetter          UnitGetter
	unitUpdater         UnitUpdater

	aliveUnitsChan chan []string
}

func newApplicationWorker(
	application string,
	brokerManagedUnits bool,
	serviceBroker ServiceBroker,
	containerBroker ContainerBroker,
	containerSpecGetter ContainerSpecGetter,
	lifeGetter LifeGetter,
	applicationGetter ApplicationGetter,
	unitGetter UnitGetter,
	unitUpdater UnitUpdater,
) (worker.Worker, error) {
	w := &applicationWorker{
		application:         application,
		brokerManagedUnits:  brokerManagedUnits,
		serviceBroker:       serviceBroker,
		containerBroker:     containerBroker,
		containerSpecGetter: containerSpecGetter,
		lifeGetter:          lifeGetter,
		applicationGetter:   applicationGetter,
		unitGetter:          unitGetter,
		unitUpdater:         unitUpdater,
		aliveUnitsChan:      make(chan []string),
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
	jujuUnitsWatcher, err := aw.unitGetter.WatchUnits(aw.application)
	if err != nil {
		return errors.Trace(err)
	}
	aw.catacomb.Add(jujuUnitsWatcher)

	brokerUnitsWatcher, err := aw.containerBroker.WatchUnits(aw.application)
	if err != nil {
		return errors.Annotatef(err, "failed to start unit watcher for %q", aw.application)
	}
	if err := aw.catacomb.Add(brokerUnitsWatcher); err != nil {
		return errors.Trace(err)
	}

	var deploymentWorker worker.Worker
	if aw.brokerManagedUnits {
		deploymentWorker, err = newDeploymentWorker(
			aw.application,
			aw.serviceBroker,
			aw.containerSpecGetter,
			aw.applicationGetter,
			aw.aliveUnitsChan)
		if err != nil {
			return errors.Trace(err)
		}
		aw.catacomb.Add(deploymentWorker)
	}
	unitWorkers := make(map[string]worker.Worker)
	aliveUnits := make(set.Strings)
	var aliveUnitsChan chan []string

	for {
		select {
		case <-aw.catacomb.Dying():
			return aw.catacomb.ErrDying()
		case units, ok := <-jujuUnitsWatcher.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			if aw.brokerManagedUnits {
				aliveUnitsChan = aw.aliveUnitsChan
			}
			for _, unitId := range units {
				unitLife, err := aw.lifeGetter.Life(unitId)
				if errors.IsNotFound(err) {
					aliveUnits.Remove(unitId)
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

				if unitLife == life.Dead {
					aliveUnits.Remove(unitId)
				} else {
					aliveUnits.Add(unitId)
				}
				if !aw.brokerManagedUnits {
					// For Juju managed units, we start a worker to manage the unit.
					if _, ok := unitWorkers[unitId]; ok || unitLife == life.Dead {
						// Already watching the unit. or we're
						// not yet watching it and it's dead.
						continue
					}
					w, err := newUnitWorker(aw.application, unitId, aw.containerBroker, aw.containerSpecGetter)
					if err != nil {
						return errors.Trace(err)
					}
					unitWorkers[unitId] = w
					aw.catacomb.Add(w)
				}
			}
		case aliveUnitsChan <- aliveUnits.Values():
			aliveUnitsChan = nil
		case _, ok := <-brokerUnitsWatcher.Changes():
			logger.Debugf("units changed: %#v", ok)
			if !ok {
				return brokerUnitsWatcher.Wait()
			}
			units, err := aw.containerBroker.Units(aw.application)
			if err != nil {
				return errors.Trace(err)
			}
			logger.Debugf("units for %v: %+v", aw.application, units)
			args := params.UpdateApplicationUnits{
				ApplicationTag: names.NewApplicationTag(aw.application).String(),
				Units:          make([]params.ApplicationUnitParams, len(units)),
			}
			for i, u := range units {
				args.Units[i] = params.ApplicationUnitParams{
					Id:      u.Id,
					Address: u.Address,
					Ports:   u.Ports,
					Status:  u.Status.Status.String(),
					Info:    u.Status.Message,
					Data:    u.Status.Data,
				}
			}
			if err := aw.unitUpdater.UpdateUnits(args); err != nil {
				return errors.Trace(err)
			}
		}
	}
}
