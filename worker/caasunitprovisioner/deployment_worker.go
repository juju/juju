// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

// deploymentWorker informs the CAAS broker of how many pods to run and their spec, and
// lets the broker figure out how to make that all happen.
type deploymentWorker struct {
	catacomb            catacomb.Catacomb
	application         string
	broker              ServiceBroker
	applicationGetter   ApplicationGetter
	containerSpecGetter ContainerSpecGetter

	aliveUnitsChan <-chan []string
}

func newDeploymentWorker(
	application string,
	broker ServiceBroker,
	containerSpecGetter ContainerSpecGetter,
	applicationGetter ApplicationGetter,
	aliveUnitsChan <-chan []string,
) (worker.Worker, error) {
	w := &deploymentWorker{
		application:         application,
		broker:              broker,
		containerSpecGetter: containerSpecGetter,
		applicationGetter:   applicationGetter,
		aliveUnitsChan:      aliveUnitsChan,
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
func (w *deploymentWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *deploymentWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *deploymentWorker) loop() error {

	var (
		aliveUnits []string
		cw         watcher.NotifyWatcher
		specChan   watcher.NotifyChannel

		currentAliveCount int
		currentSpec       string
	)

	gotSpecNotify := false
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case aliveUnits = <-w.aliveUnitsChan:
			if len(aliveUnits) > 0 && specChan == nil {
				var err error
				cw, err = w.containerSpecGetter.WatchContainerSpec(aliveUnits[0])
				if err != nil {
					return errors.Trace(err)
				}
				w.catacomb.Add(cw)
				specChan = cw.Changes()
			}
		case _, ok := <-specChan:
			if !ok {
				return errors.New("watcher closed channel")
			}
			gotSpecNotify = true
		}
		if len(aliveUnits) == 0 {
			if cw != nil {
				worker.Stop(cw)
				specChan = nil
			}
			if err := w.broker.DeleteService(w.application); err != nil {
				return errors.Trace(err)
			}
			continue
		}

		// TODO(caas) - for now, we assume all units are homogeneous
		// so we just need to get the first spec and use that one.
		if !gotSpecNotify {
			continue
		}
		unitSpec, err := w.containerSpecGetter.ContainerSpec(aliveUnits[0])
		if errors.IsNotFound(err) {
			// No container spec defined for a unit yet;
			// wait for one to be set.
			continue
		} else if err != nil {
			return errors.Trace(err)
		}

		numUnits := len(aliveUnits)
		if numUnits == currentAliveCount && unitSpec == currentSpec {
			continue
		}

		currentAliveCount = numUnits
		currentSpec = unitSpec

		appConfig, err := w.applicationGetter.ApplicationConfig(w.application)
		if err != nil {
			return errors.Trace(err)
		}
		err = w.broker.EnsureService(w.application, unitSpec, numUnits, appConfig)
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("created/updated deployment for %s for %d units", w.application, numUnits)
	}
}
