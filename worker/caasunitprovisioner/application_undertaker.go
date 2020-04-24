// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/watcher"
)

type applicationUndertaker struct {
	catacomb        catacomb.Catacomb
	application     string
	mode            caas.DeploymentMode
	serviceBroker   ServiceBroker
	containerBroker ContainerBroker

	applicationUpdater ApplicationUpdater
	logger             Logger
}

// newApplicationUndertaker is a worker which monitors the cluster for
// resources which we need to be cleaned up prior to allowing an
// application to be removed from the Juju model.
func newApplicationUndertaker(
	application string,
	mode caas.DeploymentMode,
	serviceBroker ServiceBroker,
	containerBroker ContainerBroker,
	applicationUpdater ApplicationUpdater,
	logger Logger,
) (*applicationUndertaker, error) {
	w := &applicationUndertaker{
		application:        application,
		mode:               mode,
		serviceBroker:      serviceBroker,
		containerBroker:    containerBroker,
		applicationUpdater: applicationUpdater,
		logger:             logger,
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
func (au *applicationUndertaker) Kill() {
	au.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (au *applicationUndertaker) Wait() error {
	return au.catacomb.Wait()
}

func (au *applicationUndertaker) loop() (err error) {
	var (
		brokerUnitsWatcher watcher.NotifyWatcher
		brokerUnitsChannel watcher.NotifyChannel

		appOperatorWatcher  watcher.NotifyWatcher
		appOpertatorChannel watcher.NotifyChannel
	)
	// The caas watcher can just die from underneath hence it needs to be
	// restarted all the time. So we don't abuse the catacomb by adding new
	// workers unbounded, use a defer to stop the running worker.
	defer func() {
		au.logger.Warningf("EXIT AU WORKER: %v", err)
		if brokerUnitsWatcher != nil {
			worker.Stop(brokerUnitsWatcher)
		}
		if appOperatorWatcher != nil {
			worker.Stop(appOperatorWatcher)
		}
	}()

	logger := au.logger
	// For now, we only care about workload pods and the operator;
	// these are what Juju watches to update the model and are
	// therefore what we need to wait on.
	pendingResources := set.NewStrings("pods")
	if au.mode == caas.ModeWorkload {
		pendingResources.Add("operator")
	}
	for {
		logger.Debugf("resources pending: %v", pendingResources.Values())
		if pendingResources.Size() == 0 {
			logger.Debugf("all resources gone for %v", au.application)
			err = au.applicationUpdater.ClearApplicationResources(au.application)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			return nil
		}
		var err error
		// The caas watcher can just die from underneath so recreate if needed.
		if brokerUnitsWatcher == nil {
			brokerUnitsWatcher, err = au.containerBroker.WatchUnits(au.application, au.mode)
			if err != nil {
				if strings.Contains(err.Error(), "unexpected EOF") {
					logger.Warningf("k8s cloud hosting %q has disappeared", au.application, au.mode)
					return nil
				}
				return errors.Annotatef(err, "failed to start unit watcher for %q", au.application)
			}
			brokerUnitsChannel = brokerUnitsWatcher.Changes()
		}
		if appOperatorWatcher == nil && au.mode == caas.ModeWorkload {
			appOperatorWatcher, err = au.containerBroker.WatchOperator(au.application)
			if err != nil {
				if strings.Contains(err.Error(), "unexpected EOF") {
					logger.Warningf("k8s cloud hosting %q has disappeared", au.application)
					return nil
				}
				return errors.Annotatef(err, "failed to start operator watcher for %q", au.application)
			}
			appOpertatorChannel = appOperatorWatcher.Changes()
		}

		select {
		// We must handle any processing due to application being removed prior
		// to shutdown so that we don't leave stuff running in the cloud.
		case <-au.catacomb.Dying():
			return au.catacomb.ErrDying()
		case _, ok := <-brokerUnitsChannel:
			if !ok {
				worker.Stop(brokerUnitsWatcher)
				brokerUnitsWatcher = nil
				continue
			}
			units, err := au.containerBroker.Units(au.application, au.mode)
			if err != nil {
				return errors.Trace(err)
			}
			if len(units) == 0 {
				pendingResources.Remove("pods")
			}
			continue
		case _, ok := <-appOpertatorChannel:
			if !ok {
				worker.Stop(appOperatorWatcher)
				appOperatorWatcher = nil
				continue
			}
			_, err := au.containerBroker.Operator(au.application)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			if err != nil {
				pendingResources.Remove("operator")
			}
			continue
		}
	}
}
