// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/core/life"
)

var logger = loggo.GetLogger("juju.workers.caasunitprovisioner")

// Config holds configuration for the CAAS unit provisioner worker.
type Config struct {
	ApplicationGetter  ApplicationGetter
	ApplicationUpdater ApplicationUpdater
	ServiceBroker      ServiceBroker

	ContainerBroker          ContainerBroker
	ProvisioningInfoGetter   ProvisioningInfoGetter
	ProvisioningStatusSetter ProvisioningStatusSetter
	LifeGetter               LifeGetter
	UnitUpdater              UnitUpdater
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
	if config.ProvisioningInfoGetter == nil {
		return errors.NotValidf("missing ProvisioningInfoGetter")
	}
	if config.LifeGetter == nil {
		return errors.NotValidf("missing LifeGetter")
	}
	if config.UnitUpdater == nil {
		return errors.NotValidf("missing UnitUpdater")
	}
	if config.ProvisioningStatusSetter == nil {
		return errors.NotValidf("missing ProvisioningStatusSetter")
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

	// appWorkers holds the worker created to manage each application.
	// It's defined here so that we can access it in tests.
	appWorkers map[string]*applicationWorker
	mu         sync.Mutex
}

// Kill is part of the worker.Worker interface.
func (p *provisioner) Kill() {
	p.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *provisioner) Wait() error {
	return p.catacomb.Wait()
}

// These helper methods protect the appWorkers map so we can access for testing.

func (p *provisioner) saveApplicationWorker(appName string, aw *applicationWorker) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.appWorkers == nil {
		p.appWorkers = make(map[string]*applicationWorker)
	}
	p.appWorkers[appName] = aw
}

func (p *provisioner) deleteApplicationWorker(appName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.appWorkers, appName)
}

func (p *provisioner) getApplicationWorker(appName string) (*applicationWorker, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.appWorkers) == 0 {
		return nil, false
	}
	aw, ok := p.appWorkers[appName]
	return aw, ok
}

func (p *provisioner) loop() error {
	w, err := p.config.ApplicationGetter.WatchApplications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(w); err != nil {
		return errors.Trace(err)
	}

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
					if err := p.config.ServiceBroker.UnexposeService(appId); err != nil {
						return errors.Trace(err)
					}
					if err := p.config.ServiceBroker.DeleteService(appId); err != nil {
						return errors.Trace(err)
					}
					w, ok := p.getApplicationWorker(appId)
					if ok {
						// Before stopping the application worker, inform it that
						// the app is gone so it has a chance to clean up.
						// The worker will act on the removal prior to processing the
						// Stop() request.
						if err := worker.Stop(w); err != nil {
							logger.Errorf("stopping application worker for %v: %v", appId, err)
						}
						p.deleteApplicationWorker(appId)
					}
					continue
				}
				if err != nil {
					return errors.Trace(err)
				}
				if _, ok := p.getApplicationWorker(appId); ok || appLife == life.Dead {
					// Already watching the application. or we're
					// not yet watching it and it's dead.
					continue
				}
				w, err := newApplicationWorker(
					appId,
					p.config.ServiceBroker,
					p.config.ContainerBroker,
					p.config.ProvisioningStatusSetter,
					p.config.ProvisioningInfoGetter,
					p.config.ApplicationGetter,
					p.config.ApplicationUpdater,
					p.config.UnitUpdater,
				)
				if err != nil {
					return errors.Trace(err)
				}
				p.saveApplicationWorker(appId, w)
				p.catacomb.Add(w)
			}
		}
	}
}
