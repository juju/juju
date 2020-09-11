// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallerembedded

import (
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	// "github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/caas"
	// "github.com/juju/juju/core/life"
	// "github.com/juju/juju/environs/tags"
)

type applicationWorker struct {
	catacomb       catacomb.Catacomb
	controllerUUID string
	modelUUID      string
	appName        string

	firewallerAPI CAASFirewallerAPI

	broker CAASBroker

	lifeGetter LifeGetter

	initial           bool
	previouslyExposed bool

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

func (w *applicationWorker) loop() (err error) {
	defer func() {
		// If the application has been deleted, we can return nil.
		if errors.IsNotFound(err) {
			w.logger.Debugf("caas firewaller application %v has been removed", w.appName)
			err = nil
		}
	}()
	appWatcher, err := w.firewallerAPI.WatchApplication(w.appName)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}

	// portsWatcher, err := w.firewallerAPI.WatchPorts??(w.appName)
	// if err != nil {
	// 	return errors.Trace(err)
	// }
	// if err := w.catacomb.Add(portsWatcher); err != nil {
	// 	return errors.Trace(err)
	// }

	// var appLife life.Value = life.Dead
	charmURL, err := w.firewallerAPI.ApplicationCharmURL(w.appName)
	if err != nil {
		return errors.Annotatef(err, "failed to get charm urls for application")
	}
	charmInfo, err := w.firewallerAPI.CharmInfo(charmURL.String())
	if err != nil {
		return errors.Annotatef(err, "failed to get application charm deployment metadata for %q", w.appName)
	}
	if charmInfo == nil ||
		charmInfo.Meta == nil ||
		charmInfo.Meta.Deployment == nil ||
		charmInfo.Meta.Deployment.DeploymentMode != charm.ModeEmbedded {
		return errors.Errorf("charm missing deployment mode or received non-embedded mode")
	}

	app := w.broker.Application(w.appName,
		caas.DeploymentType(charmInfo.Meta.Deployment.DeploymentType))

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}
			if err := w.processApplicationChange(app); err != nil {
				if strings.Contains(err.Error(), "unexpected EOF") {
					return nil
				}
				return errors.Trace(err)
			}
			// case portMappings, ok := <-portsWatcher.Changes():
			// 	if !ok {
			// 		return errors.New("application watcher closed")
			// 	}
			// 	app.OpenPort(p)
			// 	app.ClosePort(p)
		}
	}
}

func (w *applicationWorker) processApplicationChange(app caas.Application) (err error) {
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
	// juju expose always takes higher priority than open/close port??
	if err != nil {
		return errors.Trace(err)
	}
	if !w.initial && exposed == w.previouslyExposed {
		return nil
	}

	w.initial = false
	w.previouslyExposed = exposed
	if exposed {
		// appConfig, err := w.firewallerAPI.ApplicationConfig(w.appName)
		// if err != nil {
		// 	return errors.Trace(err)
		// }
		// resourceTags := tags.ResourceTags(
		// 	names.NewModelTag(w.modelUUID),
		// 	names.NewControllerTag(w.controllerUUID),
		// )
		if err := exposeService(app); err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	if err := unExposeService(app); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func exposeService(app caas.Application) error {
	// TODO:
	// app.UpdateService()
	return nil
}

func unExposeService(app caas.Application) error {
	// TODO:
	// app.UpdateService()
	return nil
}
