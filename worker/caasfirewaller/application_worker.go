// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"strings"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/environs/tags"
)

type applicationWorker struct {
	catacomb          catacomb.Catacomb
	controllerUUID    string
	modelUUID         string
	application       string
	applicationGetter ApplicationGetter
	serviceExposer    ServiceExposer
	lifeGetter        LifeGetter
	charmGetter       CharmGetter

	initial           bool
	previouslyExposed bool

	logger Logger
}

func newApplicationWorker(
	controllerUUID string,
	modelUUID string,
	application string,
	applicationGetter ApplicationGetter,
	applicationExposer ServiceExposer,
	lifeGetter LifeGetter,
	charmGetter CharmGetter,
	logger Logger,
) (worker.Worker, error) {
	w := &applicationWorker{
		controllerUUID:    controllerUUID,
		modelUUID:         modelUUID,
		application:       application,
		applicationGetter: applicationGetter,
		serviceExposer:    applicationExposer,
		lifeGetter:        lifeGetter,
		charmGetter:       charmGetter,
		initial:           true,
		logger:            logger,
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
			w.logger.Debugf("caas firewaller application %v has been removed", w.application)
			err = nil
		}
	}()
	appWatcher, err := w.applicationGetter.WatchApplication(w.application)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("application watcher closed")
			}

			// If charm is (now) a v2 charm, exit the worker.
			format, err := w.charmFormat()
			if errors.IsNotFound(err) {
				w.logger.Debugf("application %q no longer exists", w.application)
				return nil
			} else if err != nil {
				return errors.Trace(err)
			}
			if format >= charm.FormatV2 {
				w.logger.Debugf("application %q v1 worker got v2 charm event, stopping", w.application)
				return nil
			}

			if err := w.processApplicationChange(); err != nil {
				if strings.Contains(err.Error(), "unexpected EOF") {
					return nil
				}
				return errors.Trace(err)
			}
		}
	}
}

func (w *applicationWorker) charmFormat() (charm.Format, error) {
	charmInfo, err := w.charmGetter.ApplicationCharmInfo(w.application)
	if err != nil {
		return charm.FormatUnknown, errors.Annotatef(err, "failed to get charm info for application %q", w.application)
	}
	return charm.MetaFormat(charmInfo.Charm()), nil
}

func (w *applicationWorker) processApplicationChange() (err error) {
	defer func() {
		// Not found could be because the app got removed or there's
		// no container service created yet as the app is still being set up.
		if errors.IsNotFound(err) {
			// Perhaps the app got removed while we were processing.
			if _, err2 := w.lifeGetter.Life(w.application); err2 != nil {
				err = err2
				return
			}
			// Ignore not found error because the ip could be not ready yet at this stage.
			w.logger.Warningf("processing change for application %q, %v", w.application, err)
			err = nil
		}
	}()

	exposed, err := w.applicationGetter.IsExposed(w.application)
	if err != nil {
		return errors.Trace(err)
	}
	if !w.initial && exposed == w.previouslyExposed {
		return nil
	}

	w.initial = false
	w.previouslyExposed = exposed
	if exposed {
		appConfig, err := w.applicationGetter.ApplicationConfig(w.application)
		if err != nil {
			return errors.Trace(err)
		}
		resourceTags := tags.ResourceTags(
			names.NewModelTag(w.modelUUID),
			names.NewControllerTag(w.controllerUUID),
		)
		if err := w.serviceExposer.ExposeService(w.application, resourceTags, appConfig); err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	if err := w.serviceExposer.UnexposeService(w.application); err != nil {
		return errors.Trace(err)
	}
	return nil
}
