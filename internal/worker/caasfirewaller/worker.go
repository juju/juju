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

// applicationWorkerCreator describes a func that is capable of starting new
// application firewall worker.
type ApplicationWorkerCreator func(
	appUUID application.UUID,
) (worker.Worker, error)

// FirewallerConfig holds configuration for the CAAS unit firewaller worker.
type FirewallerConfig struct {
	ApplicationService ApplicationService
	Logger             logger.Logger
	WorkerCreator      ApplicationWorkerCreator
}

// firewaller is a worker responsible for watching applications in the model and
// ensuring they have their corresponding application firewall events handled.
type firewaller struct {
	catacomb   catacomb.Catacomb
	appService ApplicationService
	logger     logger.Logger

	appWorkers    map[application.UUID]worker.Worker
	workerCreator ApplicationWorkerCreator
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

// NewFirewallerWorker starts and returns a new CAAS firewaller worker watching
// and responding to application firewall events in the model.
func NewFirewallerWorker(config FirewallerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating worker configuration: %w", err)
	}

	p := &firewaller{
		appService:    config.ApplicationService,
		logger:        config.Logger,
		appWorkers:    make(map[application.UUID]worker.Worker),
		workerCreator: config.WorkerCreator,
	}

	err := catacomb.Invoke(catacomb.Plan{
		Name: "caas-firewaller",
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}

// ensureApplicationWorkerStarted ensures that a firewall worker exists for the
// supplied application uuid.
func (p *firewaller) ensureApplicationWorkerStarted(
	ctx context.Context, appUUID application.UUID,
) error {
	if _, ok := p.appWorkers[appUUID]; ok {
		// Already watching the application.
		return nil
	}

	p.logger.Debugf(
		ctx, "creating application %q caas firewaller worker", appUUID,
	)

	w, err := p.workerCreator(appUUID)
	if err != nil {
		return errors.Errorf(
			"starting new application %q firewall worker: %w",
			appUUID, err,
		)
	}

	err = p.catacomb.Add(w)
	if err != nil {
		if werr := worker.Stop(w); werr != nil {
			return errors.Errorf(
				"stopping application %q worker after failing to add to catacomb: %w",
				appUUID, errors.Join(err, werr),
			)
		}
		return errors.Errorf(
			"adding application %q worker to catacomb: %w", appUUID, err,
		)
	}

	p.appWorkers[appUUID] = w

	return nil
}

// ensureApplicationWorkerStopped ensures that any existing firewall worker for
// the supplied application uuid is stopped and removed.
func (p *firewaller) ensureApplicationWorkerStopped(
	ctx context.Context, appUUID application.UUID,
) error {
	w, exists := p.appWorkers[appUUID]
	if !exists {
		return nil
	}

	p.logger.Debugf(
		ctx, "removing application %q caas firewaller worker", appUUID,
	)

	defer delete(p.appWorkers, appUUID)
	err := worker.Stop(w)
	if err != nil {
		return errors.Errorf("stopping application %q worker: %w", appUUID, err)
	}

	return nil
}

// Kill is part of the worker.Worker interface.
func (p *firewaller) Kill() {
	p.catacomb.Kill(nil)
}

// observeApplicationFirewallChange observes and responds to a single event for
// an application uuid. If the application exists and is alive this func will
// make sure that a worker is running for the application. If the application
// is dead or removed and previosuly created workers will be stopped and
// removed.
//
// Should the application not be using the v2 charm format it will be ignored
// with no further processing done.
func (p *firewaller) observeApplicationFirewallChange(
	ctx context.Context, appUUID application.UUID,
) error {
	appLife, err := p.appService.GetApplicationLife(ctx, appUUID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		// Application no longer exists, make sure that any workers are stopped.
		return p.ensureApplicationWorkerStopped(ctx, appUUID)
	} else if err != nil {
		return errors.Errorf(
			"getting current life for application %q: %w",
			appUUID, err,
		)
	}

	if appLife == life.Dead {
		// Application is dead, make sure that any workers are stopped.
		return p.ensureApplicationWorkerStopped(ctx, appUUID)
	}

	// We only ever need to perform this check if no worker already exists for
	// the application. If a worker already exists then the implication exists.
	isV2, err := p.isV2Charm(ctx, appUUID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		// Application no longer exists, make sure that any workers are stopped.
		return p.ensureApplicationWorkerStopped(ctx, appUUID)
	} else if err != nil {
		return errors.Errorf(
			"determing charm v2 format for application %q: %w", appUUID, err,
		)
	}

	if !isV2 {
		p.logger.Debugf(
			ctx,
			"application %q is for a v1 charm, no caas firewaller required",
			appUUID,
		)
		return nil
	}

	return p.ensureApplicationWorkerStarted(ctx, appUUID)
}

// Wait is part of the worker.Worker interface.
func (p *firewaller) Wait() error {
	return p.catacomb.Wait()
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

// isV2Charm works out if the charm backing the application uuid is on the v2
// format returning true or false.
//
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application does not exist.
func (p *firewaller) isV2Charm(
	ctx context.Context, appUUID application.UUID,
) (bool, error) {
	if _, ok := p.appWorkers[appUUID]; ok {
		// If the application uuid already has a started worker then it can be
		// inferred that it is a V2 charm. This avoids excess trips to the
		// service.
		return true, nil
	}

	ch, _, err := p.appService.GetCharmByApplicationUUID(ctx, appUUID)
	if err != nil {
		return false, errors.Errorf(
			"getting charm information: %w", err,
		)
	}

	if charm.MetaFormat(ch) < charm.FormatV2 {
		return false, nil
	}
	return true, nil
}
