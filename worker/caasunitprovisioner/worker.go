// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.workers.caasunitprovisioner")

// Config holds configuration for the CAAS unit provisioner worker.
type Config struct {
	ApplicationGetter  ApplicationGetter
	ApplicationUpdater ApplicationUpdater
	ServiceBroker      ServiceBroker

	ContainerBroker ContainerBroker
	PodSpecGetter   PodSpecGetter
	LifeGetter      LifeGetter
	UnitGetter      UnitGetter
	UnitUpdater     UnitUpdater
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.ApplicationGetter == nil {
		return errors.NotValidf("missing ApplicationGetter")
	}
	if config.ApplicationUpdater == nil {
		return errors.NotValidf("missing ApplicationUpdater")
	}
	if config.ServiceBroker == nil {
		return errors.NotValidf("missing ServiceBroker")
	}
	if config.ContainerBroker == nil {
		return errors.NotValidf("missing ContainerBroker")
	}
	if config.PodSpecGetter == nil {
		return errors.NotValidf("missing PodSpecGetter")
	}
	if config.LifeGetter == nil {
		return errors.NotValidf("missing LifeGetter")
	}
	if config.UnitGetter == nil {
		return errors.NotValidf("missing UnitGetter")
	}
	if config.UnitUpdater == nil {
		return errors.NotValidf("missing UnitUpdater")
	}
	return nil
}

// NewWorker starts and returns a new CAAS unit provisioner worker.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	p := &provisioner{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}

type provisioner struct {
	catacomb catacomb.Catacomb
	config   Config

	appRemoved chan struct{}
}

// Kill is part of the worker.Worker interface.
func (p *provisioner) Kill() {
	p.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *provisioner) Wait() error {
	return p.catacomb.Wait()
}

func (p *provisioner) loop() error {
	w, err := p.config.ApplicationGetter.WatchApplications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(w); err != nil {
		return errors.Trace(err)
	}

	// The channel is unbuffered to that we block until
	// requests are processed.
	p.appRemoved = make(chan struct{})
	appWorkers := make(map[string]worker.Worker)
	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case apps, ok := <-w.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			for _, appId := range apps {
				appLife, err := p.config.LifeGetter.Life(appId)
				if errors.IsNotFound(err) {
					// Once an application is deleted, remove the k8s service and ingress resources.
					if err := p.config.ContainerBroker.UnexposeService(appId); err != nil {
						return errors.Trace(err)
					}
					if err := p.config.ContainerBroker.DeleteService(appId); err != nil {
						return errors.Trace(err)
					}
					w, ok := appWorkers[appId]
					if ok {
						// Before stopping the application worker, inform it that
						// the app is gone so it has a chance to clean up.
						// The worker will act on the removed prior to processing the
						// Stop() request.
						p.appRemoved <- struct{}{}
						if err := worker.Stop(w); err != nil {
							logger.Errorf("stopping application worker for %v: %v", appId, err)
						}
						delete(appWorkers, appId)
					}
					continue
				}
				if err != nil {
					return errors.Trace(err)
				}
				if _, ok := appWorkers[appId]; ok || appLife == life.Dead {
					// Already watching the application. or we're
					// not yet watching it and it's dead.
					continue
				}
				cfg, err := p.config.ApplicationGetter.ApplicationConfig(appId)
				if err != nil {
					return errors.Trace(err)
				}
				jujuManagedUnits := cfg.GetBool(caas.JujuManagedUnits, false)
				w, err := newApplicationWorker(
					appId,
					p.appRemoved,
					jujuManagedUnits,
					p.config.ServiceBroker,
					p.config.ContainerBroker,
					p.config.PodSpecGetter,
					p.config.LifeGetter,
					p.config.ApplicationGetter,
					p.config.ApplicationUpdater,
					p.config.UnitGetter,
					p.config.UnitUpdater,
				)
				if err != nil {
					return errors.Trace(err)
				}
				appWorkers[appId] = w
				p.catacomb.Add(w)
			}
		}
	}
}
