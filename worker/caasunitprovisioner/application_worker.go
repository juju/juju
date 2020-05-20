// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"reflect"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

type applicationWorker struct {
	catacomb        catacomb.Catacomb
	application     string
	mode            caas.DeploymentMode
	serviceBroker   ServiceBroker
	containerBroker ContainerBroker

	provisioningStatusSetter ProvisioningStatusSetter
	provisioningInfoGetter   ProvisioningInfoGetter
	applicationGetter        ApplicationGetter
	applicationUpdater       ApplicationUpdater
	unitUpdater              UnitUpdater

	logger Logger
}

func newApplicationWorker(
	application string,
	mode caas.DeploymentMode,
	serviceBroker ServiceBroker,
	containerBroker ContainerBroker,
	provisioningStatusSetter ProvisioningStatusSetter,
	provisioningInfoGetter ProvisioningInfoGetter,
	applicationGetter ApplicationGetter,
	applicationUpdater ApplicationUpdater,
	unitUpdater UnitUpdater,
	logger Logger,
) (*applicationWorker, error) {
	w := &applicationWorker{
		application:              application,
		mode:                     mode,
		serviceBroker:            serviceBroker,
		containerBroker:          containerBroker,
		provisioningStatusSetter: provisioningStatusSetter,
		provisioningInfoGetter:   provisioningInfoGetter,
		applicationGetter:        applicationGetter,
		applicationUpdater:       applicationUpdater,
		unitUpdater:              unitUpdater,
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
func (aw *applicationWorker) Kill() {
	aw.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (aw *applicationWorker) Wait() error {
	return aw.catacomb.Wait()
}

func (aw *applicationWorker) loop() error {
	if aw.mode == caas.ModeWorkload {
		deploymentWorker, err := newDeploymentWorker(
			aw.application,
			aw.provisioningStatusSetter,
			aw.serviceBroker,
			aw.provisioningInfoGetter,
			aw.applicationGetter,
			aw.applicationUpdater,
			aw.logger,
		)
		if err != nil {
			return errors.Trace(err)
		}
		aw.catacomb.Add(deploymentWorker)
	}

	var (
		brokerUnitsWatcher watcher.NotifyWatcher
		brokerUnitsChannel watcher.NotifyChannel

		appOperatorWatcher watcher.NotifyWatcher
		appOperatorChannel watcher.NotifyChannel

		appDeploymentWatcher watcher.NotifyWatcher
		appDeploymentChannel watcher.NotifyChannel
	)
	// The caas watcher can just die from underneath hence it needs to be
	// restarted all the time. So we don't abuse the catacomb by adding new
	// workers unbounded, use a defer to stop the running worker.
	defer func() {
		if brokerUnitsWatcher != nil {
			worker.Stop(brokerUnitsWatcher)
		}
		if appOperatorWatcher != nil {
			worker.Stop(appOperatorWatcher)
		}
		if appDeploymentWatcher != nil {
			worker.Stop(appDeploymentWatcher)
		}
	}()

	// Cache the last reported status information
	// so we only report true changes.
	lastReportedStatus := make(map[string]status.StatusInfo)
	lastReportedScale := -1
	initialOperatorEvent := true
	logger := aw.logger
	for {
		var err error
		// The caas watcher can just die from underneath so recreate if needed.
		if brokerUnitsWatcher == nil {
			brokerUnitsWatcher, err = aw.containerBroker.WatchUnits(aw.application, aw.mode)
			if err != nil {
				if strings.Contains(err.Error(), "unexpected EOF") {
					logger.Warningf("k8s cloud hosting %q has disappeared", aw.application, aw.mode)
					return nil
				}
				return errors.Annotatef(err, "failed to start unit watcher for %q", aw.application)
			}
			brokerUnitsChannel = brokerUnitsWatcher.Changes()
		}
		if appOperatorWatcher == nil && aw.mode == caas.ModeWorkload {
			appOperatorWatcher, err = aw.containerBroker.WatchOperator(aw.application)
			if err != nil {
				if strings.Contains(err.Error(), "unexpected EOF") {
					logger.Warningf("k8s cloud hosting %q has disappeared", aw.application)
					return nil
				}
				return errors.Annotatef(err, "failed to start operator watcher for %q", aw.application)
			}
			appOperatorChannel = appOperatorWatcher.Changes()
		}
		if appDeploymentWatcher == nil {
			appDeploymentWatcher, err = aw.serviceBroker.WatchService(aw.application, aw.mode)
			if err != nil {
				if strings.Contains(err.Error(), "unexpected EOF") {
					logger.Warningf("k8s cloud hosting %q has disappeared", aw.application, aw.mode)
					return nil
				}
				return errors.Annotatef(err, "failed to start deployment watcher for %q", aw.application)
			}
			appDeploymentChannel = appDeploymentWatcher.Changes()
		}

		select {
		// We must handle any processing due to application being removed prior
		// to shutdown so that we don't leave stuff running in the cloud.
		case <-aw.catacomb.Dying():
			return aw.catacomb.ErrDying()
		case _, ok := <-brokerUnitsChannel:
			logger.Debugf("units changed: %#v", ok)
			if !ok {
				logger.Debugf("%v", brokerUnitsWatcher.Wait())
				worker.Stop(brokerUnitsWatcher)
				brokerUnitsWatcher = nil
				continue
			}
			service, err := aw.serviceBroker.GetService(aw.application, aw.mode, false)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			logger.Debugf("service for %v(%v): %+v", aw.application, aw.mode, service)
			if err := aw.clusterChanged(service, lastReportedStatus, true); err != nil {
				// TODO(caas): change the shouldSetScale to false here once appDeploymentWatcher can get all events from k8s.
				return errors.Trace(err)
			}
		case _, ok := <-appDeploymentChannel:
			logger.Debugf("deployment changed: %#v", ok)
			if !ok {
				logger.Debugf("%v", appDeploymentWatcher.Wait())
				worker.Stop(appDeploymentWatcher)
				appDeploymentWatcher = nil
				continue
			}
			service, err := aw.serviceBroker.GetService(aw.application, aw.mode, false)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			haveNewStatus := true
			if service.Id != "" {
				// update svc info (addresses etc.) cloudservices.
				err = updateApplicationService(
					names.NewApplicationTag(aw.application), service, aw.applicationUpdater,
				)
				if errors.IsForbidden(err) {
					// ignore errors raised from SetScale because disordered events could happen often.
					logger.Warningf("%v", err)
				} else if err != nil {
					return errors.Trace(err)
				}
				lastStatus, ok := lastReportedStatus[service.Id]
				lastReportedStatus[service.Id] = service.Status
				if ok {
					// If we've seen the same status value previously,
					// report as unknown as this value is ignored.
					if reflect.DeepEqual(lastStatus, service.Status) {
						service.Status = status.StatusInfo{
							Status: status.Unknown,
						}
						haveNewStatus = false
					}
				}
			}
			if service != nil && service.Scale != nil {
				if *service.Scale == lastReportedScale && !haveNewStatus {
					continue
				}
				lastReportedScale = *service.Scale
			}
			if err := aw.clusterChanged(service, lastReportedStatus, true); err != nil {
				return errors.Trace(err)
			}
		case _, ok := <-appOperatorChannel:
			if !ok {
				logger.Debugf("%v", appOperatorWatcher.Wait())
				worker.Stop(appOperatorWatcher)
				appOperatorWatcher = nil
				continue
			}
			logger.Debugf("operator update for %v", aw.application)
			operator, err := aw.containerBroker.Operator(aw.application)
			if errors.IsNotFound(err) {
				if initialOperatorEvent {
					initialOperatorEvent = false
					continue
				}
				logger.Debugf("pod not found for application %q", aw.application)
				if err := aw.provisioningStatusSetter.SetOperatorStatus(aw.application, status.Terminated, "", nil); err != nil {
					return errors.Trace(err)
				}
			} else if err != nil {
				return errors.Trace(err)
			} else {
				if err := aw.provisioningStatusSetter.SetOperatorStatus(aw.application, operator.Status.Status, operator.Status.Message, operator.Status.Data); err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (aw *applicationWorker) clusterChanged(
	service *caas.Service,
	lastReportedStatus map[string]status.StatusInfo,
	shouldSetScale bool,
) error {
	units, err := aw.containerBroker.Units(aw.application, aw.mode)
	if err != nil {
		return errors.Trace(err)
	}
	serviceStatus := service.Status
	var scale *int
	var generation *int64
	if service != nil && shouldSetScale {
		generation = service.Generation
		scale = service.Scale
	}
	args := params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag(aw.application).String(),
		Scale:          scale,
		Generation:     generation,
		Status: params.EntityStatus{
			Status: serviceStatus.Status,
			Info:   serviceStatus.Message,
			Data:   serviceStatus.Data,
		},
	}
	for _, u := range units {
		// For pods managed by the substrate, any marked as dying
		// are treated as non-existing.
		if u.Dying {
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
		unitParams := params.ApplicationUnitParams{
			ProviderId: u.Id,
			Address:    u.Address,
			Ports:      u.Ports,
			Stateful:   u.Stateful,
			Status:     unitStatus.Status.String(),
			Info:       unitStatus.Message,
			Data:       unitStatus.Data,
		}
		// Fill in any filesystem info for volumes attached to the unit.
		// A unit will not become active until all required volumes are
		// provisioned, so it makes sense to send this information along
		// with the units to which they are attached.
		for _, info := range u.FilesystemInfo {
			unitParams.FilesystemInfo = append(unitParams.FilesystemInfo, params.KubernetesFilesystemInfo{
				StorageName:  info.StorageName,
				FilesystemId: info.FilesystemId,
				Size:         info.Size,
				MountPoint:   info.MountPoint,
				ReadOnly:     info.ReadOnly,
				Status:       info.Status.Status.String(),
				Info:         info.Status.Message,
				Data:         info.Status.Data,
				Volume: params.KubernetesVolumeInfo{
					VolumeId:   info.Volume.VolumeId,
					Size:       info.Volume.Size,
					Persistent: info.Volume.Persistent,
					Status:     info.Volume.Status.Status.String(),
					Info:       info.Volume.Status.Message,
					Data:       info.Volume.Status.Data,
				},
			})
		}
		args.Units = append(args.Units, unitParams)
	}
	appUnitInfo, err := aw.unitUpdater.UpdateUnits(args)
	if err != nil {
		// We can ignore not found errors as the worker will get stopped anyway.
		// We can also ignore Forbidden errors raised from SetScale because disordered events could happen often.
		if !errors.IsForbidden(err) && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		aw.logger.Warningf("update units %v", err)
	}

	if appUnitInfo != nil {
		for _, unitInfo := range appUnitInfo.Units {
			unit, err := names.ParseUnitTag(unitInfo.UnitTag)
			if err != nil {
				return errors.Trace(err)
			}
			err = aw.containerBroker.AnnotateUnit(aw.application, aw.mode, unitInfo.ProviderId, unit)
			if errors.IsNotFound(err) {
				continue
			} else if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}
