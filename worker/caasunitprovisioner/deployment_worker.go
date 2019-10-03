// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
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
	desiredScale := 0
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-appScaleWatcher.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			var err error
			desiredScale, err = w.applicationGetter.ApplicationScale(w.application)
			if err != nil {
				return errors.Trace(err)
			}
			logger.Debugf("desiredScale changed to %d", desiredScale)
			if desiredScale > 0 && specChan == nil {
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
		if desiredScale > 0 && !gotSpecNotify {
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
		if desiredScale == 0 {
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
		if desiredScale == currentScale && specStr == currentSpec {
			continue
		}

		currentScale = desiredScale
		currentSpec = specStr

		appConfig, err := w.applicationGetter.ApplicationConfig(w.application)
		if err != nil {
			return errors.Trace(err)
		}
		spec, err := k8sspecs.ParsePodSpec(specStr)
		if err != nil {
			return errors.Annotate(err, "cannot parse pod spec")
		}

		serviceParams := &caas.ServiceParams{
			PodSpec:      spec,
			Constraints:  info.Constraints,
			ResourceTags: info.Tags,
			Filesystems:  info.Filesystems,
			Devices:      info.Devices,
			Deployment: caas.DeploymentParams{
				DeploymentType: caas.DeploymentType(info.DeploymentInfo.DeploymentType),
				ServiceType:    caas.ServiceType(info.DeploymentInfo.ServiceType),
			},
		}
		err = w.broker.EnsureService(w.application, w.provisioningStatusSetter.SetOperatorStatus, serviceParams, desiredScale, appConfig)
		if err != nil {
			// Some errors we don't want to exit the worker.
			if k8sprovider.MaskError(err) {
				logger.Errorf(err.Error())
				continue
			}
			return errors.Trace(err)
		}
		logger.Debugf("ensured deployment for %s for %v units", w.application, desiredScale)
		if !serviceUpdated && !spec.OmitServiceFrontend {
			service, err := w.broker.GetService(w.application, false)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Annotate(err, "cannot get new service details")
			}
			if err = updateApplicationService(
				names.NewApplicationTag(w.application), service, w.applicationUpdater,
			); err != nil {
				return errors.Trace(err)
			}
			serviceUpdated = true
		}
	}
}

func updateApplicationService(appTag names.ApplicationTag, svc *caas.Service, updater ApplicationUpdater) error {
	if svc == nil || svc.Id == "" {
		return nil
	}
	return updater.UpdateApplicationService(
		params.UpdateApplicationServiceArg{
			ApplicationTag: appTag.String(),
			ProviderId:     svc.Id,
			Addresses:      params.FromProviderAddresses(svc.Addresses...),
			Scale:          svc.Scale,
			Generation:     svc.Generation,
		},
	)
}
