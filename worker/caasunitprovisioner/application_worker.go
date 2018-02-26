// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

type applicationWorker struct {
	catacomb         catacomb.Catacomb
	application      string
	jujuManagedUnits bool
	serviceBroker    ServiceBroker
	containerBroker  ContainerBroker

	containerSpecGetter ContainerSpecGetter
	lifeGetter          LifeGetter
	applicationGetter   ApplicationGetter
	applicationUpdater  ApplicationUpdater
	unitGetter          UnitGetter
	unitUpdater         UnitUpdater

	aliveUnitsChan chan []string
	appRemoved     chan struct{}
}

func newApplicationWorker(
	application string,
	appRemoved chan struct{},
	jujuManagedUnits bool,
	serviceBroker ServiceBroker,
	containerBroker ContainerBroker,
	containerSpecGetter ContainerSpecGetter,
	lifeGetter LifeGetter,
	applicationGetter ApplicationGetter,
	applicationUpdater ApplicationUpdater,
	unitGetter UnitGetter,
	unitUpdater UnitUpdater,
) (worker.Worker, error) {
	w := &applicationWorker{
		application:         application,
		jujuManagedUnits:    jujuManagedUnits,
		serviceBroker:       serviceBroker,
		containerBroker:     containerBroker,
		containerSpecGetter: containerSpecGetter,
		lifeGetter:          lifeGetter,
		applicationGetter:   applicationGetter,
		applicationUpdater:  applicationUpdater,
		unitGetter:          unitGetter,
		unitUpdater:         unitUpdater,
		aliveUnitsChan:      make(chan []string),
		appRemoved:          appRemoved,
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

	deploymentWorker, err := newDeploymentWorker(
		aw.application,
		aw.jujuManagedUnits,
		aw.serviceBroker,
		aw.containerSpecGetter,
		aw.applicationGetter,
		aw.applicationUpdater,
		aw.aliveUnitsChan)
	if err != nil {
		return errors.Trace(err)
	}
	aw.catacomb.Add(deploymentWorker)
	unitWorkers := make(map[string]worker.Worker)
	aliveUnits := make(set.Strings)
	var (
		aliveUnitsChan     chan []string
		brokerUnitsWatcher watcher.NotifyWatcher
	)
	// The caas watcher can just die from underneath us hence it needs to be
	// restarted all the time. So we don't abuse the catacomb by adding new
	// workers unbounded, use use a defer to stop the running worker.
	defer func() {
		if brokerUnitsWatcher != nil {
			worker.Stop(brokerUnitsWatcher)
		}
	}()

	// Cache the last reported status information
	// so we only report true changes.
	lastReportedStatus := make(map[string]status.StatusInfo)

	for {
		// The caas watcher can just die from underneath us so recreate if needed.
		if brokerUnitsWatcher == nil {
			brokerUnitsWatcher, err = aw.containerBroker.WatchUnits(aw.application)
			if err != nil {
				return errors.Annotatef(err, "failed to start unit watcher for %q", aw.application)
			}
		}
		select {
		// We must handle any processing due to application being removed prior
		// to shutdown so that we don't leave stuff running in the cloud.
		case <-aw.appRemoved:
			if !aw.jujuManagedUnits {
				continue
			}
			// Application has been removed, ensure all units are stopped
			// before this worker is killed.
			for unitId, w := range unitWorkers {
				if err := aw.containerBroker.DeleteUnit(unitId); err != nil {
					logger.Errorf("error deleting unit %v of removed application: %v", unitId, err)
				}
				worker.Stop(w)
			}
		case <-aw.catacomb.Dying():
			return aw.catacomb.ErrDying()
		case aliveUnitsChan <- aliveUnits.Values():
			aliveUnitsChan = nil
		case _, ok := <-brokerUnitsWatcher.Changes():
			logger.Debugf("units changed: %#v", ok)
			if !ok {
				logger.Warningf("%v", brokerUnitsWatcher.Wait())
				worker.Stop(brokerUnitsWatcher)
				brokerUnitsWatcher = nil
				continue
			}
			units, err := aw.containerBroker.Units(aw.application)
			if err != nil {
				return errors.Trace(err)
			}
			logger.Debugf("units for %v: %+v", aw.application, units)
			args := params.UpdateApplicationUnits{
				ApplicationTag: names.NewApplicationTag(aw.application).String(),
			}
			for _, u := range units {
				// For pods managed by the substrate, any marked as dying
				// are treated as non-existing.
				if u.Dying && !aw.jujuManagedUnits {
					continue
				}
				unitStatus := u.Status
				lastStatus, ok := lastReportedStatus[u.Id]
				lastReportedStatus[u.Id] = unitStatus
				if ok {
					// If we've seen the same status value previously,
					// report as unknown as this value is ignored.
					if reflect.DeepEqual(lastStatus, unitStatus) {
						unitStatus = status.StatusInfo{
							Status: status.Unknown,
						}
					}
				}
				args.Units = append(args.Units, params.ApplicationUnitParams{
					ProviderId: u.Id,
					UnitTag:    u.UnitTag,
					Address:    u.Address,
					Ports:      u.Ports,
					Status:     unitStatus.Status.String(),
					Info:       unitStatus.Message,
					Data:       unitStatus.Data,
				})
			}
			if err := aw.unitUpdater.UpdateUnits(args); err != nil {
				// We can ignore not found errors as the worker will get stopped anyway.
				if !errors.IsNotFound(err) {
					return errors.Trace(err)
				}
			}
		case units, ok := <-jujuUnitsWatcher.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			aliveUnitsChan = aw.aliveUnitsChan
			for _, unitId := range units {
				unitLife, err := aw.lifeGetter.Life(unitId)
				if err != nil && !errors.IsNotFound(err) {
					return errors.Trace(err)
				}
				if errors.IsNotFound(err) || unitLife == life.Dead {
					aliveUnits.Remove(unitId)
					w, ok := unitWorkers[unitId]
					if ok {
						if err := worker.Stop(w); err != nil {
							logger.Errorf("stopping unit worker for %v: %v", unitId, err)
						}
						delete(unitWorkers, unitId)
					}
				} else {
					aliveUnits.Add(unitId)
				}
				if aw.jujuManagedUnits {
					// Remove any deleted unit.
					if !aliveUnits.Contains(unitId) {
						if err := aw.containerBroker.DeleteUnit(unitId); err != nil {
							return errors.Trace(err)
						}
						logger.Debugf("deleted unit %s", unitId)
						continue
					}
					// Start a worker to manage any new units.
					if _, ok := unitWorkers[unitId]; ok || unitLife == life.Dead {
						// Already watching the unit. or we're
						// not yet watching it and it's dead.
						continue
					}
					w, err := newUnitWorker(
						aw.application, unitId, aw.containerBroker, aw.containerSpecGetter, aw.lifeGetter)
					if err != nil {
						return errors.Trace(err)
					}
					unitWorkers[unitId] = w
					aw.catacomb.Add(w)
				}
			}
		}
	}
}
