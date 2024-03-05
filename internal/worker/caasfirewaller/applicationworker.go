// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
)

type applicationWorker struct {
	catacomb       catacomb.Catacomb
	controllerUUID string
	modelUUID      string
	appName        string

	firewallerAPI CAASFirewallerAPI

	broker         CAASBroker
	portMutator    PortMutator
	serviceUpdater ServiceUpdater

	appWatcher   watcher.NotifyWatcher
	portsWatcher watcher.StringsWatcher

	lifeGetter LifeGetter

	initial           bool
	previouslyExposed bool

	currentPorts network.GroupedPortRanges

	logger Logger
}

func newApplicationWorker(
	controllerUUID string,
	modelUUID string,
	appName string,
	firewallerAPI CAASFirewallerAPI,
	broker CAASBroker,
	lifeGetter LifeGetter,
	logger Logger,
) (worker.Worker, error) {
	w := &applicationWorker{
		controllerUUID: controllerUUID,
		modelUUID:      modelUUID,
		appName:        appName,
		firewallerAPI:  firewallerAPI,
		broker:         broker,
		lifeGetter:     lifeGetter,
		initial:        true,
		logger:         logger,
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

func (w *applicationWorker) setUp() (err error) {
	w.appWatcher, err = w.firewallerAPI.WatchApplication(w.appName)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(w.appWatcher); err != nil {
		return errors.Trace(err)
	}

	w.portsWatcher, err = w.firewallerAPI.WatchOpenedPorts()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(w.portsWatcher); err != nil {
		return errors.Trace(err)
	}

	// TODO(sidecar): support deployment other than statefulset
	app := w.broker.Application(w.appName, caas.DeploymentStateful)
	w.portMutator = app
	w.serviceUpdater = app

	if w.currentPorts, err = w.firewallerAPI.GetOpenedPorts(w.appName); err != nil {
		return errors.Annotatef(err, "failed to get initial openned ports for application")
	}

	return nil
}

func (w *applicationWorker) loop() (err error) {
	defer func() {
		// If the application has been deleted, we can return nil.
		if errors.Is(err, errors.NotFound) {
			w.logger.Debugf("sidecar caas firewaller application %v has been removed", w.appName)
			err = nil
		}
	}()

	if err = w.setUp(); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-w.appWatcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}
			// We know this is a v2 charm at this point, because this child
			// worker is only ever started for v2 charms.
			if err := w.onApplicationChanged(); err != nil {
				if strings.Contains(err.Error(), "unexpected EOF") {
					return nil
				}
				return errors.Trace(err)
			}
		case _, ok := <-w.portsWatcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}
			// TODO(sidecar): implement portWatcher to return application names having port changes,
			/*
				if !sets.NewString(changes...).Contains(w.appName){continue}
			*/
			if err := w.onPortChanged(); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func toServicePorts(in network.GroupedPortRanges) []caas.ServicePort {
	ports := in.UniquePortRanges()
	network.SortPortRanges(ports)
	out := make([]caas.ServicePort, len(ports))
	for i, p := range ports {
		out[i] = caas.ServicePort{
			// k8s complains about `/`.
			Name:       strings.Replace(p.String(), "/", "-", -1),
			Port:       p.FromPort,
			TargetPort: p.ToPort,
			Protocol:   p.Protocol,
		}
	}
	return out
}

func (w *applicationWorker) onPortChanged() error {
	changedPortRanges, err := w.firewallerAPI.GetOpenedPorts(w.appName)
	if err != nil {
		return err
	}
	w.logger.Tracef("current port for app %q, %v", w.appName, w.currentPorts)
	w.logger.Tracef("port changed for app %q, %v", w.appName, changedPortRanges)
	if w.currentPorts.EqualTo(changedPortRanges) {
		w.logger.Debugf("no port changes for app %q", w.appName)
		return nil
	}

	err = w.portMutator.UpdatePorts(toServicePorts(changedPortRanges), false)
	if errors.Is(err, errors.NotFound) {
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "cannot update service port for application %q", w.appName)
	}

	w.currentPorts = changedPortRanges
	return nil
}

func (w *applicationWorker) onApplicationChanged() (err error) {
	defer func() {
		// Not found could be because the app got removed or there's
		// no container service created yet as the app is still being set up.
		if errors.Is(err, errors.NotFound) {
			// Perhaps the app got removed while we were processing.
			if _, err2 := w.lifeGetter.Life(w.appName); err2 != nil {
				err = err2
				return
			}
			// Ignore not found error because the ip could be not ready yet at this stage.
			w.logger.Warningf("processing change for application %q, %v", w.appName, err)
			err = nil
		}
	}()

	exposed, err := w.firewallerAPI.IsExposed(w.appName)
	if err != nil {
		return errors.Trace(err)
	}
	if !w.initial && exposed == w.previouslyExposed {
		return nil
	}

	w.initial = false
	w.previouslyExposed = exposed
	if exposed {
		return errors.Trace(exposeService(w.serviceUpdater))
	}
	return errors.Trace(unExposeService(w.serviceUpdater))
}

func exposeService(app ServiceUpdater) error {
	// TODO(sidecar): implement expose once it's modelled.
	// app.UpdateService()
	return nil
}

func unExposeService(app ServiceUpdater) error {
	// TODO(sidecar): implement un-expose once it's modelled.
	// app.UpdateService()
	return nil
}
