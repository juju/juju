// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

// applicationWorkerCreator describes a func that is capable of starting new
// application firewall worker.
type AppFirewallerWokerCreator func(
	appUUID application.UUID,
) (worker.Worker, error)

// FirewallerConfig holds configuration for the CAAS unit firewaller worker.
type FirewallerConfig struct {
	ApplicationService ApplicationService
	Clock              clock.Clock
	Logger             logger.Logger
	WorkerCreator      AppFirewallerWokerCreator
}

// firewaller is a worker responsible for watching applications in the model and
// ensuring they have their corresponding application firewall events handled.
type firewaller struct {
	*worker.Runner
	catacomb   catacomb.Catacomb
	appService ApplicationService
	logger     logger.Logger

	//appWorkers    map[application.UUID]worker.Worker
	workerCreator AppFirewallerWokerCreator
}

// applicationWorkerNamePrefix is the prefix used for naming application
// firewaller workers made by [firewaller]. This name is important as it is used
// within log messages by the runner when managing the worker.
const applicationWorkerNamePrefix = "caas-firewaller-application-"

// Kill is part of the worker.Worker interface.
func (p *firewaller) Kill() {
	p.catacomb.Kill(nil)
}

// loop runs indefinitely until this worker is stopped. For each watched
// application in the model it ensures that a corresponding application firewall
// worker is started or stopped based on the life of the application.
func (p *firewaller) loop() error {
	ctx := p.catacomb.Context(context.Background())

	w, err := p.appService.WatchApplications(ctx)
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
				appUUID := application.UUID(app)
				err := p.observeApplicationFirewallChange(ctx, appUUID)
				if err != nil {
					return err
				}
			}
		}
	}
}

// NewFirewallerWorker starts and returns a new CAAS firewaller worker watching
// and responding to application firewall events in the model.
func NewFirewallerWorker(config FirewallerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating worker configuration: %w", err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Clock: config.Clock,
		Name:  "application-caas-firewall-workers",
		IsFatal: func(e error) bool {
			// It is not fatal if the error is an application not found error.
			// We don't expect this error to be leaked outwards but just in case
			// it does.
			return !errors.Is(e, applicationerrors.ApplicationNotFound)
		},
		Logger: internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Errorf(
			"creating new application firewall runner: %w", err,
		)
	}

	p := &firewaller{
		Runner:        runner,
		appService:    config.ApplicationService,
		logger:        config.Logger,
		workerCreator: config.WorkerCreator,
	}

	err = catacomb.Invoke(catacomb.Plan{
		// This is important, we make the catacomb responsible for the life of
		// [runner]. From this point out we don't have to worry about its life
		// management.
		Init: []worker.Worker{runner},
		Name: "caas-firewaller",
		Site: &p.catacomb,
		Work: p.loop,
	})
	if err != nil {
		return nil, errors.Errorf(
			"invoking new catacomb for worker: %w", err,
		)
	}

	return p, nil
}

// observeApplicationFirewallChange observes and responds to a single event for
// an application uuid. If the application exists and is alive this func will
// make sure that a worker is running for the application. If the application
// is dead or removed and previously created workers will be stopped and
// removed.
func (p *firewaller) observeApplicationFirewallChange(
	ctx context.Context, appUUID application.UUID,
) error {
	appLife, err := p.appService.GetApplicationLife(ctx, appUUID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		// Application no longer exists, make sure that any workers are stopped.
		return p.stopApplicationWorker(ctx, appUUID)
	} else if err != nil {
		return errors.Errorf(
			"getting current life for application %q: %w",
			appUUID, err,
		)
	}

	if appLife == life.Dead {
		// Application is dead, make sure that any workers are stopped.
		return p.stopApplicationWorker(ctx, appUUID)
	}

	return p.startApplicationWorker(ctx, appUUID)
}

// startApplicationWorker ensures that a firewall worker exists for the
// supplied application uuid.
func (p *firewaller) startApplicationWorker(
	ctx context.Context, appUUID application.UUID,
) error {
	p.logger.Debugf(
		ctx, "creating application %q caas firewaller worker", appUUID,
	)

	err := p.Runner.StartWorker(
		ctx,
		applicationWorkerNamePrefix+appUUID.String(),
		func(context.Context) (worker.Worker, error) {
			return p.workerCreator(appUUID)
		},
	)
	if errors.Is(err, coreerrors.AlreadyExists) {
		// Worker already exists for the application nothing more to do.
		return nil
	} else if err != nil {
		return errors.Errorf(
			"starting firewaller worker for application %q: %w", appUUID, err,
		)
	}

	return nil
}

// stopApplicationWorker ensures that any existing firewall worker for
// the supplied application uuid is stopped and removed.
func (p *firewaller) stopApplicationWorker(
	ctx context.Context, appUUID application.UUID,
) error {
	p.logger.Debugf(
		ctx, "removing application %q caas firewaller worker", appUUID,
	)

	err := p.Runner.StopWorker(applicationWorkerNamePrefix + appUUID.String())
	if err != nil {
		return errors.Errorf(
			"stopping firewaller worker for application %q: %w", appUUID, err,
		)
	}

	return nil
}

// Validate takes the worker [FirewallerConfig] and checks that each field is set to an
// acceptable value for the worker to operate.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when a required field has an invalid value.
func (config FirewallerConfig) Validate() error {
	if config.ApplicationService == nil {
		return errors.New("not valid nil ApplicationService").Add(
			coreerrors.NotValid,
		)
	}
	if config.Clock == nil {
		return errors.New("not valid nil Clock").Add(
			coreerrors.NotValid,
		)
	}
	if config.Logger == nil {
		return errors.New("not valid nil Logger").Add(
			coreerrors.NotValid,
		)
	}
	if config.WorkerCreator == nil {
		return errors.New("not valid nil WorkerCreator").Add(
			coreerrors.NotValid,
		)
	}
	return nil
}

// Wait is part of the worker.Worker interface.
func (p *firewaller) Wait() error {
	return p.catacomb.Wait()
}
