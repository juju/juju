// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallerembedded

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/core/life"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one passed as manifold config.
var logger interface{}

// Config holds configuration for the CAAS unit firewaller worker.
type Config struct {
	ControllerUUID string
	ModelUUID      string
	Facade         CAASFirewallerAPI
	LifeGetter     LifeGetter
	Broker         CAASBroker
	Logger         Logger
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.ControllerUUID == "" {
		return errors.NotValidf("missing ControllerUUID")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("missing ModelUUID")
	}
	if config.Facade == nil {
		return errors.NotValidf("missing Facade")
	}
	if config.Broker == nil {
		return errors.NotValidf("missing Broker")
	}
	if config.LifeGetter == nil {
		return errors.NotValidf("missing LifeGetter")
	}
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

// NewWorker starts and returns a new CAAS unit firewaller worker.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	p := &firewaller{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}

type firewaller struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill is part of the worker.Worker interface.
func (p *firewaller) Kill() {
	p.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *firewaller) Wait() error {
	return p.catacomb.Wait()
}

func (p *firewaller) loop() error {
	logger := p.config.Logger
	w, err := p.config.Facade.WatchApplications()
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
			for _, appName := range apps {
				appLife, err := p.config.LifeGetter.Life(appName)
				if errors.IsNotFound(err) {
					w, ok := appWorkers[appName]
					if ok {
						if err := worker.Stop(w); err != nil {
							logger.Errorf("error stopping caas firewaller: %v", err)
						}
						delete(appWorkers, appName)
					}
					continue
				}
				if err != nil {
					return errors.Trace(err)
				}
				if _, ok := appWorkers[appName]; ok || appLife == life.Dead {
					// Already watching the application. or we're
					// not yet watching it and it's dead.
					continue
				}
				w, err := newApplicationWorker(
					p.config.ControllerUUID,
					p.config.ModelUUID,
					appName,
					p.config.Facade,
					p.config.Broker,
					p.config.LifeGetter,
					logger,
				)
				if err != nil {
					return errors.Trace(err)
				}
				appWorkers[appName] = w
				p.catacomb.Add(w)
			}
		}
	}
}
