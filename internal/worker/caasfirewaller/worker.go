// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// Config holds configuration for the CAAS unit firewaller worker.
type Config struct {
	ControllerUUID     string
	ModelUUID          string
	PortService        PortService
	ApplicationService ApplicationService
	Broker             CAASBroker
	Logger             logger.Logger
}

// Validate takes the worker [Config] and checks that each field is set to an
// acceptable value for the worker to operate.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when a required field has an invalid value.
func (config Config) Validate() error {
	if config.ControllerUUID == "" {
		return errors.New("not valid empty ControllerUUID").Add(
			coreerrors.NotValid,
		)
	}
	if config.ModelUUID == "" {
		return errors.New("not valid empty ModelUUID").Add(
			coreerrors.NotValid,
		)
	}
	if config.Broker == nil {
		return errors.New("not valid nil Broker").Add(
			coreerrors.NotValid,
		)
	}
	if config.PortService == nil {
		return errors.New("not valid nil PortService").Add(
			coreerrors.NotValid,
		)
	}
	if config.ApplicationService == nil {
		return errors.New("not valid nil ApplicationService").Add(
			coreerrors.NotValid,
		)
	}
	if config.Logger == nil {
		return errors.New("not valid nil Logger").Add(
			coreerrors.NotValid,
		)
	}
	return nil
}

// NewWorker starts and returns a new CAAS unit firewaller worker.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating worker configuration: %w", err)
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

	appWorkers           map[application.UUID]worker.Worker
	newApplicationWorker applicationWorkerCreator
}

type applicationWorkerCreator func(
	controllerUUID string,
	modelUUID string,
	appUUID application.UUID,
	portService PortService,
	applicationService ApplicationService,
	broker CAASBroker,
	logger logger.Logger,
) (worker.Worker, error)

func newFirewaller(config Config, f applicationWorkerCreator) *firewaller {
	return &firewaller{
		config:               config,
		appWorkers:           make(map[application.UUID]worker.Worker),
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
	w, err := p.config.ApplicationService.WatchApplications(ctx)
	if err != nil {
		return errors.Errorf("getting application watcher: %w", err)
	}
	if err := p.catacomb.Add(w); err != nil {
		return errors.Errorf(
			"adding application watcher to the worker's catacomb: %w", err,
		)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()

		case apps, ok := <-w.Changes():
			if !ok {
				return errors.New(
					"application watcher's channel closed unexpectedly",
				)
			}

			for _, app := range apps {
				appUUID, err := application.ParseUUID(app)
				if err != nil {
					return errors.Errorf(
						"parsing recieved watcher application %q uuid: %w",
						app, err,
					)
				}

				// If charm is a v1 charm, skip processing.
				format, err := p.charmFormat(ctx, appUUID)
				if errors.Is(err, coreerrors.NotFound) {
					p.config.Logger.Debugf(ctx, "application %q no longer exists", appUUID)
					continue
				} else if err != nil {
					return errors.Errorf(
						"getting recieved watcher application %q charm format: %w",
						appUUID, err,
					)
				}

				if format < charm.FormatV2 {
					p.config.Logger.Tracef(ctx, "v2 caasfirewaller got event for v1 app %q, skipping", appUUID)
					continue
				}

				appLife, err := p.config.ApplicationService.GetApplicationLife(ctx, appUUID)
				if errors.Is(err, applicationerrors.ApplicationNotFound) || appLife == life.Dead {
					w, ok := p.appWorkers[appUUID]
					if ok {
						if err := worker.Stop(w); err != nil {
							logger.Errorf(ctx, "error stopping caas firewaller: %v", err)
						}
						delete(p.appWorkers, appUUID)
					}
					continue
				}
				if err != nil {
					return errors.Errorf(
						"getting recieved watcher application %q current life: %w",
						appUUID, err,
					)
				}
				if _, ok := p.appWorkers[appUUID]; ok {
					// Already watching the application.
					continue
				}

				w, err := p.newApplicationWorker(
					p.config.ControllerUUID,
					p.config.ModelUUID,
					appUUID,
					p.config.PortService,
					p.config.ApplicationService,
					p.config.Broker,
					logger,
				)
				if err != nil {
					return errors.Errorf(
						"starting watcher application %q firewall worker: %w",
						appUUID, err,
					)
				}
				if err := p.catacomb.Add(w); err != nil {
					if err2 := worker.Stop(w); err2 != nil {
						logger.Errorf(ctx, "error stopping caas application worker: %v", err2)
					}
					return errors.Errorf(
						"adding watcher application %q firewall worker to catacomb: %w",
						appUUID, err,
					)
				}
				p.appWorkers[appUUID] = w
			}
		}
	}
}

// charmFormat returns the [charm.Format] for the supplied application uuid.
func (p *firewaller) charmFormat(ctx context.Context, appUUID application.UUID) (charm.Format, error) {
	ch, _, err := p.config.ApplicationService.GetCharmByApplicationUUID(ctx, appUUID)
	if err != nil {
		return charm.FormatUnknown, errors.Errorf(
			"getting charm information: %w", err,
		)
	}
	return charm.MetaFormat(ch), nil
}

func (p *firewaller) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(p.catacomb.Context(context.Background()))
}
