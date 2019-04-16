// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/api/caasunitprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/watcher"
)

// deploymentWorker informs the CAAS broker of how many pods to run and their spec, and
// lets the broker figure out how to make that all happen.
type deploymentWorker struct {
	catacomb                 catacomb.Catacomb
	application              string
	provisioningStatusSetter ProvisioningStatusSetter
	broker                   ServiceBroker
	applicationGetter        ApplicationGetter
	applicationUpdater       ApplicationUpdater
	provisioningInfoGetter   ProvisioningInfoGetter
}

func newDeploymentWorker(
	application string,
	provisioningStatusSetter ProvisioningStatusSetter,
	broker ServiceBroker,
	provisioningInfoGetter ProvisioningInfoGetter,
	applicationGetter ApplicationGetter,
	applicationUpdater ApplicationUpdater,
) (worker.Worker, error) {
	w := &deploymentWorker{
		application:              application,
		provisioningStatusSetter: provisioningStatusSetter,
		broker:                   broker,
		provisioningInfoGetter:   provisioningInfoGetter,
		applicationGetter:        applicationGetter,
		applicationUpdater:       applicationUpdater,
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
	appScaleWatcher, err := w.applicationGetter.WatchApplicationScale(w.application)
	if err != nil {
		return errors.Trace(err)
	}
	w.catacomb.Add(appScaleWatcher)

	var (
		cw       watcher.NotifyWatcher
		specChan watcher.NotifyChannel

		currentScale int
		currentSpec  string
	)

	gotSpecNotify := false
	serviceUpdated := false
	scale := 0
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-appScaleWatcher.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			var err error
			scale, err = w.applicationGetter.ApplicationScale(w.application)
			if err != nil {
				return errors.Trace(err)
			}
			if scale > 0 && specChan == nil {
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
		if scale > 0 && !gotSpecNotify {
			continue
		}
		info, err := w.provisioningInfoGetter.ProvisioningInfo(w.application)
		noUnits := errors.Cause(err) == caasunitprovisioner.ErrNoUnits
		if errors.IsNotFound(err) {
			// No pod spec defined for a unit yet;
			// wait for one to be set.
			continue
		} else if err != nil && !noUnits {
			return errors.Trace(err)
		}
		if scale == 0 || noUnits {
			if cw != nil {
				worker.Stop(cw)
				specChan = nil
			}
			logger.Debugf("no units for %v", w.application)
			err = w.broker.EnsureService(w.application, w.provisioningStatusSetter.SetOperatorStatus, &caas.ServiceParams{}, 0, nil)
			if err != nil {
				return errors.Trace(err)
			}
			currentScale = 0
			continue
		}
		specStr := info.PodSpec

		if scale == currentScale && specStr == currentSpec {
			continue
		}

		currentScale = scale
		currentSpec = specStr

		appConfig, err := w.applicationGetter.ApplicationConfig(w.application)
		if err != nil {
			return errors.Trace(err)
		}
		spec, err := w.broker.Provider().ParsePodSpec(specStr)
		if err != nil {
			return errors.Annotate(err, "cannot parse pod spec")
		}
		if len(spec.CustomResourceDefinitions) > 0 {
			err = w.broker.EnsureCustomResourceDefinition(w.application, spec)
			if err != nil {
				return errors.Trace(err)
			}
			logger.Debugf("created/updated custom resource definition for %q.", w.application)
		}
		serviceParams := &caas.ServiceParams{
			PodSpec:      spec,
			Constraints:  info.Constraints,
			ResourceTags: info.Tags,
			Filesystems:  info.Filesystems,
			Devices:      info.Devices,
		}
		err = w.broker.EnsureService(w.application, w.provisioningStatusSetter.SetOperatorStatus, serviceParams, currentScale, appConfig)
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("created/updated deployment for %s for %v units", w.application, currentScale)
		if !serviceUpdated && !spec.OmitServiceFrontend {
			service, err := w.broker.GetService(w.application, false)
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
