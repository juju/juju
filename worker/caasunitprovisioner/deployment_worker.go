// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

// deploymentWorker informs the CAAS broker of how many pods to run and their spec, and
// lets the broker figure out how to make that all happen.
type deploymentWorker struct {
	catacomb               catacomb.Catacomb
	application            string
	broker                 ServiceBroker
	applicationGetter      ApplicationGetter
	applicationUpdater     ApplicationUpdater
	provisioningInfoGetter ProvisioningInfoGetter

	aliveUnitsChan <-chan []string
}

func newDeploymentWorker(
	application string,
	broker ServiceBroker,
	provisioningInfoGetter ProvisioningInfoGetter,
	applicationGetter ApplicationGetter,
	applicationUpdater ApplicationUpdater,
	aliveUnitsChan <-chan []string,
) (worker.Worker, error) {
	w := &deploymentWorker{
		application:            application,
		broker:                 broker,
		provisioningInfoGetter: provisioningInfoGetter,
		applicationGetter:      applicationGetter,
		applicationUpdater:     applicationUpdater,
		aliveUnitsChan:         aliveUnitsChan,
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
	serviceUpdated := false
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case aliveUnits = <-w.aliveUnitsChan:
			if len(aliveUnits) > 0 && specChan == nil {
				var err error
				cw, err = w.provisioningInfoGetter.WatchPodSpec(w.application)
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
			continue
		}

		if !gotSpecNotify {
			continue
		}
		info, err := w.provisioningInfoGetter.ProvisioningInfo(w.application)
		if errors.IsNotFound(err) {
			// No pod spec defined for a unit yet;
			// wait for one to be set.
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
		specStr := info.PodSpec

		numUnits := len(aliveUnits)
		if numUnits == currentAliveCount && specStr == currentSpec {
			continue
		}

		currentAliveCount = numUnits
		currentSpec = specStr

		appConfig, err := w.applicationGetter.ApplicationConfig(w.application)
		if err != nil {
			return errors.Trace(err)
		}
		spec, err := w.broker.Provider().ParsePodSpec(specStr)
		if err != nil {
			return errors.Annotate(err, "cannot parse pod spec")
		}

		serviceParams := &caas.ServiceParams{
			PodSpec:      spec,
			Constraints:  info.Constraints,
			ResourceTags: info.Tags,
			Filesystems:  info.Filesystems,
			Devices:      info.Devices,
		}
		err = w.broker.EnsureService(w.application, serviceParams, numUnits, appConfig)
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("created/updated deployment for %s for %v units", w.application, aliveUnits)
		if !serviceUpdated && !spec.OmitServiceFrontend {
			// TODO(caas) - add a service watcher
			service, err := w.broker.Service(w.application)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Annotate(err, "cannot get new service details")
			}
			err = w.applicationUpdater.UpdateApplicationService(params.UpdateApplicationServiceArg{
				ApplicationTag: names.NewApplicationTag(w.application).String(),
				ProviderId:     service.Id,
				Addresses:      params.FromNetworkAddresses(service.Addresses...),
			})
			if err != nil {
				return errors.Trace(err)
			}
			serviceUpdated = true
		}
	}
}
