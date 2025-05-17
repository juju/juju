// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	apicaasunitprovisioner "github.com/juju/juju/api/controller/caasunitprovisioner"
	"github.com/juju/juju/caas"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
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
	logger                   Logger
}

func newDeploymentWorker(
	application string,
	provisioningStatusSetter ProvisioningStatusSetter,
	broker ServiceBroker,
	provisioningInfoGetter ProvisioningInfoGetter,
	applicationGetter ApplicationGetter,
	applicationUpdater ApplicationUpdater,
	logger Logger,
) (worker.Worker, error) {
	w := &deploymentWorker{
		application:              application,
		provisioningStatusSetter: provisioningStatusSetter,
		broker:                   broker,
		provisioningInfoGetter:   provisioningInfoGetter,
		applicationGetter:        applicationGetter,
		applicationUpdater:       applicationUpdater,
		logger:                   logger,
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
	_ = w.catacomb.Add(appScaleWatcher)

	var (
		pw            watcher.NotifyWatcher
		provisionChan watcher.NotifyChannel

		currentScale int
		currentInfo  *apicaasunitprovisioner.ProvisioningInfo
	)

	gotSpecNotify := false
	serviceUpdated := false
	desiredScale := 0
	logger := w.logger
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
			if desiredScale > 0 && provisionChan == nil {
				var err error
				pw, err = w.provisioningInfoGetter.WatchPodSpec(w.application)
				if err != nil {
					return errors.Trace(err)
				}
				_ = w.catacomb.Add(pw)
				provisionChan = pw.Changes()
			}
		case _, ok := <-provisionChan:
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
			if pw != nil {
				_ = worker.Stop(pw)
				provisionChan = nil
			}
			logger.Debugf("no units for %v", w.application)
			err = w.broker.EnsureService(w.application, w.provisioningStatusSetter.SetOperatorStatus, &caas.ServiceParams{}, 0, nil)
			if err != nil {
				return errors.Trace(err)
			}
			currentScale = 0
			continue
		}

		if desiredScale == currentScale && isProvisionInfoEqual(info, currentInfo) {
			continue
		}

		// We need to disallow updates that k8s does not yet support,
		// eg changing the filesystem or device directives, or deployment info.
		// TODO(wallyworld) - support resizing of existing storage.
		if currentInfo != nil {
			var unsupportedReason string
			if !reflect.DeepEqual(info.DeploymentInfo, currentInfo.DeploymentInfo) {
				unsupportedReason = "k8s does not support updating deployment info"
			} else if !reflect.DeepEqual(info.Filesystems, currentInfo.Filesystems) {
				unsupportedReason = "k8s does not support updating storage"
			} else if !reflect.DeepEqual(info.Devices, currentInfo.Devices) {
				unsupportedReason = "k8s does not support updating devices"
			}

			if unsupportedReason != "" {
				if err = w.provisioningStatusSetter.SetOperatorStatus(
					w.application,
					status.Error,
					unsupportedReason,
					nil,
				); err != nil {
					return errors.Trace(err)
				}
				continue
			}
		}

		currentScale = desiredScale
		currentInfo = info

		appConfig, err := w.applicationGetter.ApplicationConfig(w.application)
		if err != nil {
			return errors.Trace(err)
		}

		serviceParams, err := provisionInfoToServiceParams(info)
		if err != nil {
			return errors.Trace(err)
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
		if serviceParams.PodSpec == nil {
			continue
		}
		if !serviceUpdated && !serviceParams.PodSpec.OmitServiceFrontend {
			service, err := w.broker.GetService(w.application, caas.ModeWorkload, false)
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

func provisionInfoToServiceParams(info *apicaasunitprovisioner.ProvisioningInfo) (serviceParams *caas.ServiceParams, err error) {
	if len(info.PodSpec) > 0 && len(info.RawK8sSpec) > 0 {
		// This should never happen.
		return nil, errors.NewForbidden(nil, "either PodSpec or RawK8sSpec can be set for each application, but not both")
	}

	serviceParams = &caas.ServiceParams{
		Constraints:          info.Constraints,
		ResourceTags:         info.Tags,
		Filesystems:          info.Filesystems,
		Devices:              info.Devices,
		ImageDetails:         info.ImageDetails,
		CharmModifiedVersion: info.CharmModifiedVersion,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentType(info.DeploymentInfo.DeploymentType),
			ServiceType:    caas.ServiceType(info.DeploymentInfo.ServiceType),
		},
		StorageID: info.StorageID,
	}
	if len(info.PodSpec) > 0 {
		if serviceParams.PodSpec, err = k8sspecs.ParsePodSpec(info.PodSpec); err != nil {
			return nil, errors.Annotate(err, "cannot parse pod spec")
		}
	} else if len(info.RawK8sSpec) > 0 {
		if serviceParams.RawK8sSpec, err = k8sspecs.ParseRawK8sSpec(info.RawK8sSpec); err != nil {
			return nil, errors.Annotate(err, "cannot parse raw k8s spec")
		}
	}
	return serviceParams, nil
}

// isProvisionInfoChanged checks if podspec or raw k8s spec changed or not.
func isProvisionInfoEqual(newInfo, oldInfo *apicaasunitprovisioner.ProvisioningInfo) bool {
	if newInfo == nil && oldInfo == nil {
		return true
	} else if newInfo == nil || oldInfo == nil {
		return false
	}

	return newInfo.PodSpec == oldInfo.PodSpec &&
		newInfo.RawK8sSpec == oldInfo.RawK8sSpec &&
		newInfo.CharmModifiedVersion == oldInfo.CharmModifiedVersion
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
