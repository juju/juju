// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallerembedded

import (
	"reflect"
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

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

	currentPorts portRanges

	logger Logger
}

type portRanges map[network.PortRange]bool

func newPortRanges(in []network.PortRange) portRanges {
	out := make(portRanges)
	for _, p := range in {
		out[p] = true
	}
	return out
}

func (pg portRanges) equal(in portRanges) bool {
	if len(pg) != len(in) {
		return false
	}
	return reflect.DeepEqual(pg, in)
}

func (pg portRanges) toServicePorts() []caas.ServicePort {
	var out []caas.ServicePort
	for p := range pg {
		out = append(out, caas.ServicePort{
			// k8s complains about `/`.
			Name:       strings.Replace(p.String(), "/", "-", -1),
			Port:       p.FromPort,
			TargetPort: p.ToPort,
			Protocol:   p.Protocol,
		})
	}
	return out
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

	charmInfo, err := w.firewallerAPI.ApplicationCharmInfo(w.appName)
	if err != nil {
		return errors.Annotatef(err, "failed to get application charm deployment metadata for %q", w.appName)
	}
	if charmInfo == nil ||
		charmInfo.Meta == nil ||
		charmInfo.Meta.Deployment == nil ||
		charmInfo.Meta.Deployment.DeploymentMode != charm.ModeEmbedded {
		// return errors.Errorf("charm missing deployment mode or received non-embedded mode")
	}

	app := w.broker.Application(w.appName, caas.DeploymentType(charmInfo.Meta.Deployment.DeploymentType))
	w.portMutator = app
	w.serviceUpdater = app

	ports, err := w.firewallerAPI.GetApplicationOpenedPorts(w.appName)
	if err != nil {
		return errors.Annotatef(err, "failed to get initial openned ports for application")
	}
	// Currently k8s application does not support endpoints, so there is always only one portranges under `""` endpoint.
	w.currentPorts = newPortRanges(ports.UniquePortRanges())
	return nil
}

func (w *applicationWorker) loop() (err error) {
	defer func() {
		// If the application has been deleted, we can return nil.
		if errors.IsNotFound(err) {
			w.logger.Debugf("embedded caas firewaller application %v has been removed", w.appName)
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
			// TODO(embedded): implement portWatcher to return application names having port changes,
			/*
				if !sets.NewString(changes...).Contains(w.appName){continue}
			*/
			if err := w.onPortChanged(); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (w *applicationWorker) onPortChanged() (err error) {
	ports, err := w.firewallerAPI.GetApplicationOpenedPorts(w.appName)
	if err != nil {
		return err
	}
	changedPortRanges := newPortRanges(ports.UniquePortRanges())
	w.logger.Warningf("onPortChanged w.currentPorts %#v, changedPortRanges %#v", w.currentPorts, changedPortRanges)
	if w.currentPorts.equal(changedPortRanges) {
		w.logger.Debugf("no port changes for app %q", w.appName)
		return nil
	}
	w.currentPorts = changedPortRanges
	w.initial = false
	err = w.portMutator.UpdatePorts(w.currentPorts.toServicePorts(), false)
	if err != nil {
		w.logger.Warningf("onPortChanged err %#v, err %q", err, err.Error())
	}
	if errors.IsNotFound(err) {
		return nil
	}
	return errors.Annotatef(err, "updating service port for application %q", w.appName)
}

func (w *applicationWorker) onApplicationChanged() (err error) {
	w.logger.Warningf("onApplicationChanged")
	defer func() {
		// Not found could be because the app got removed or there's
		// no container service created yet as the app is still being set up.
		if errors.IsNotFound(err) {
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
	// TODO(embedded): implement expose once it's modelled.
	// app.UpdateService()
	return nil
}

func unExposeService(app ServiceUpdater) error {
	// TODO(embedded): implement un-expose once it's modelled.
	// app.UpdateService()
	return nil
}
