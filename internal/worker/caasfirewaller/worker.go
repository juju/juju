// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charm"
)

// Config holds configuration for the CAAS unit firewaller worker.
type Config struct {
	ControllerUUID     string
	ModelUUID          string
	FirewallerAPI      CAASFirewallerAPI
	LifeGetter         LifeGetter
	PortService        PortService
	ApplicationService ApplicationService
	Broker             CAASBroker
	Logger             logger.Logger
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.ControllerUUID == "" {
		return errors.NotValidf("missing ControllerUUID")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("missing ModelUUID")
	}
	if config.FirewallerAPI == nil {
		return errors.NotValidf("missing FirewallerAPI")
	}
	if config.Broker == nil {
		return errors.NotValidf("missing Broker")
	}
	if config.PortService == nil {
		return errors.NotValidf("missing PortService")
	}
	if config.ApplicationService == nil {
		return errors.NotValidf("missing ApplicationService")
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
	p := newFirewaller(config, newApplicationWorker)
	err := catacomb.Invoke(catacomb.Plan{
		Name: "caas-firewaller",
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}

type firewaller struct {
	catacomb catacomb.Catacomb
	config   Config

	appWorkers           map[string]worker.Worker
	newApplicationWorker applicationWorkerCreator
}

type applicationWorkerCreator func(
	controllerUUID string,
	modelUUID string,
	appName string,
	appUUID application.ID,
	firewallerAPI CAASFirewallerAPI,
	portService PortService,
	broker CAASBroker,
	lifeGetter LifeGetter,
	logger logger.Logger,
) (worker.Worker, error)

func newFirewaller(config Config, f applicationWorkerCreator) *firewaller {
	return &firewaller{
		config:               config,
		appWorkers:           make(map[string]worker.Worker),
		newApplicationWorker: f,
	}
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
	ctx, cancel := p.scopedContext()
	defer cancel()

	logger := p.config.Logger
	w, err := p.config.FirewallerAPI.WatchApplications(ctx)
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
			for _, appName := range apps {
				// If charm is a v1 charm, skip processing.
				format, err := p.charmFormat(ctx, appName)
				if errors.Is(err, errors.NotFound) {
					p.config.Logger.Debugf(ctx, "application %q no longer exists", appName)
					continue
				} else if err != nil {
					return errors.Trace(err)
				}
				if format < charm.FormatV2 {
					p.config.Logger.Tracef(ctx, "v2 caasfirewaller got event for v1 app %q, skipping", appName)
					continue
				}

				appLife, err := p.config.LifeGetter.Life(ctx, appName)
				if errors.Is(err, errors.NotFound) || appLife == life.Dead {
					w, ok := p.appWorkers[appName]
					if ok {
						if err := worker.Stop(w); err != nil {
							logger.Errorf(ctx, "error stopping caas firewaller: %v", err)
						}
						delete(p.appWorkers, appName)
					}
					continue
				}
				if err != nil {
					return errors.Trace(err)
				}
				if _, ok := p.appWorkers[appName]; ok {
					// Already watching the application.
					continue
				}

				appUUID, err := p.config.ApplicationService.GetApplicationIDByName(ctx, appName)
				if err != nil {
					return errors.Trace(err)
				}
				w, err := p.newApplicationWorker(
					p.config.ControllerUUID,
					p.config.ModelUUID,
					appName,
					appUUID,
					p.config.FirewallerAPI,
					p.config.PortService,
					p.config.Broker,
					p.config.LifeGetter,
					logger,
				)
				if err != nil {
					return errors.Trace(err)
				}
				if err := p.catacomb.Add(w); err != nil {
					if err2 := worker.Stop(w); err2 != nil {
						logger.Errorf(ctx, "error stopping caas application worker: %v", err2)
					}
					return errors.Trace(err)
				}
				p.appWorkers[appName] = w
			}
		}
	}
}

func (p *firewaller) charmFormat(ctx context.Context, appName string) (charm.Format, error) {
	charmInfo, err := p.config.FirewallerAPI.ApplicationCharmInfo(ctx, appName)
	if err != nil {
		return charm.FormatUnknown, errors.Annotatef(err, "failed to get charm info for application %q", appName)
	}
	return charm.MetaFormat(charmInfo.Charm()), nil
}

func (p *firewaller) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(p.catacomb.Context(context.Background()))
}
