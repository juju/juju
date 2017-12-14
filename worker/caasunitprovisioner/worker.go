// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.workers.caasunitprovisioner")

// Config holds configuration for the CAAS unit provisioner worker.
type Config struct {
	// BrokerManagedUnits is true if the CAAS substrate ensures the
	// required number of units are running, rather than Juju having to do it.
	BrokerManagedUnits bool

	ApplicationGetter ApplicationGetter
	ServiceBroker     ServiceBroker

	ContainerBroker     ContainerBroker
	ContainerSpecGetter ContainerSpecGetter
	LifeGetter          LifeGetter
	UnitGetter          UnitGetter
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.ApplicationGetter == nil {
		return errors.NotValidf("missing ApplicationGetter")
	}
	if config.ServiceBroker == nil {
		return errors.NotValidf("missing ServiceBroker")
	}
	if config.ContainerBroker == nil {
		return errors.NotValidf("missing ContainerBroker")
	}
	if config.ContainerSpecGetter == nil {
		return errors.NotValidf("missing ContainerSpecGetter")
	}
	if config.LifeGetter == nil {
		return errors.NotValidf("missing LifeGetter")
	}
	if config.UnitGetter == nil {
		return errors.NotValidf("missing UnitGetter")
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
					w, ok := appWorkers[appId]
					if ok {
						if err := worker.Stop(w); err != nil {
							return errors.Trace(err)
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
				w, err := newApplicationWorker(
					appId,
					p.config.BrokerManagedUnits,
					p.config.ServiceBroker,
					p.config.ContainerBroker,
					p.config.ContainerSpecGetter,
					p.config.LifeGetter,
					p.config.ApplicationGetter,
					p.config.UnitGetter,
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
