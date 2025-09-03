// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationofferer

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/crossmodelrelation"
	domainlife "github.com/juju/juju/domain/life"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

// ReportableWorker is an interface that allows a worker to be reported
// on by the engine.
type ReportableWorker interface {
	worker.Worker
	Report() map[string]any
}

// RemoteApplicationWorker is an interface that defines the methods that a
// remote application worker must implement to be managed by the Worker.
type RemoteApplicationWorker interface {
	// ApplicationName returns the application name for the remote application
	// worker.
	ApplicationName() string

	// OfferUUID returns the offer UUID for the remote application worker.
	OfferUUID() string

	// ConsumeVersion returns the consume version for the remote application
	// worker.
	ConsumeVersion() int
}

// RemoteModelRelationsClient instances publish local relation changes to the
// model hosting the remote application involved in the relation, and also watches
// for remote relation changes which are then pushed to the local model.
type RemoteModelRelationsClient interface {
	// RegisterRemoteRelations sets up the remote model to participate
	// in the specified relations.
	RegisterRemoteRelations(_ context.Context, relations ...params.RegisterRemoteRelationArg) ([]params.RegisterRemoteRelationResult, error)

	// PublishRelationChange publishes relation changes to the
	// model hosting the remote application involved in the relation.
	PublishRelationChange(context.Context, params.RemoteRelationChangeEvent) error

	// WatchRelationChanges returns a watcher that notifies of changes
	// to the units in the remote model for the relation with the
	// given remote token. We need to pass the application token for
	// the case where we're talking to a v1 API and the client needs
	// to convert RelationUnitsChanges into RemoteRelationChangeEvents
	// as they come in.
	WatchRelationChanges(_ context.Context, relationToken, applicationToken string, macs macaroon.Slice) (apiwatcher.RemoteRelationWatcher, error)

	// WatchRelationSuspendedStatus starts a RelationStatusWatcher for watching the
	// relations of each specified application in the remote model.
	WatchRelationSuspendedStatus(_ context.Context, arg params.RemoteEntityArg) (watcher.RelationStatusWatcher, error)

	// WatchOfferStatus starts an OfferStatusWatcher for watching the status
	// of the specified offer in the remote model.
	WatchOfferStatus(_ context.Context, arg params.OfferArg) (watcher.OfferStatusWatcher, error)

	// WatchConsumedSecretsChanges starts a watcher for any changes to secrets
	// consumed by the specified application.
	WatchConsumedSecretsChanges(ctx context.Context, applicationToken, relationToken string, mac *macaroon.Macaroon) (watcher.SecretsRevisionWatcher, error)
}

// CrossModelRelationService is an interface that defines the methods for
// managing cross-model relations directly on the local model database.
type CrossModelRelationService interface {
	// WatchRemoteApplicationOfferrers watches the changes to remote
	// application consumers and notifies the worker of any changes.
	WatchRemoteApplicationOfferrers(ctx context.Context) (watcher.NotifyWatcher, error)

	// GetRemoteApplicationOfferrers returns the current state of all remote
	// application consumers in the local model.
	GetRemoteApplicationOfferrers(context.Context) ([]crossmodelrelation.RemoteApplicationOfferrer, error)
}

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID                  model.UUID
	CrossModelRelationService  CrossModelRelationService
	RemoteRelationClientGetter RemoteRelationClientGetter
	NewRemoteApplicationWorker NewRemoteApplicationWorkerFunc
	Clock                      clock.Clock
	Logger                     logger.Logger
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.CrossModelRelationService == nil {
		return errors.NotValidf("nil CrossModelRelationService")
	}
	if config.RemoteRelationClientGetter == nil {
		return errors.NotValidf("nil RemoteRelationClientGetter")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Worker manages relations and associated settings where
// one end of the relation is a remote application.
type Worker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner

	config Config

	crossModelRelationService CrossModelRelationService

	logger logger.Logger
}

// New returns a Worker backed by config, or an error.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:   "remote-relations",
		Clock:  config.Clock,
		Logger: internalworker.WrapLogger(config.Logger),

		// One of the remote application workers failing should not
		// prevent the others from running.
		IsFatal: func(error) bool { return false },

		// For any failures, try again in 15 seconds.
		RestartDelay: 15 * time.Second,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		runner: runner,
		config: config,

		crossModelRelationService: config.CrossModelRelationService,
		logger:                    config.Logger,
	}

	err = catacomb.Invoke(catacomb.Plan{
		Name: "remote-relations",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.runner},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Kill is defined on worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker.
func (w *Worker) Wait() error {
	err := w.catacomb.Wait()
	if err != nil {
		w.logger.Errorf(context.Background(), "error in top level remote relations worker: %v", err)
	}
	return err
}

func (w *Worker) loop() (err error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	watcher, err := w.crossModelRelationService.WatchRemoteApplicationOfferrers(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-watcher.Changes():
			if !ok {
				return errors.New("change channel closed")
			}

			if err := w.handleApplicationChanges(ctx); err != nil {
				return err
			}
		}
	}
}

func (w *Worker) handleApplicationChanges(ctx context.Context) error {
	w.logger.Debugf(ctx, "processing remote application changes")

	// Fetch the current state of each of the remote applications that have
	// changed.
	results, err := w.crossModelRelationService.GetRemoteApplicationOfferrers(ctx)
	if err != nil {
		return errors.Annotate(err, "querying remote applications")
	}

	witnessed := make(map[string]struct{})
	for _, remoteApp := range results {
		if remoteApp.Life == domainlife.Dead {
			// If the remote application is dead, then we won't witness the
			// application, so we can skip it, causing it to be removed and
			// stopped.
			continue
		}

		appName := remoteApp.ApplicationName

		// We've witnessed the application, so we need to either start a new
		// worker or recreate it depending on if the offer has changed.
		witnessed[remoteApp.ApplicationName] = struct{}{}

		// Now check to see if the offer has changed for the remote application.
		if offerChanged, err := w.hasRemoteAppChanged(remoteApp); err != nil {
			return errors.Annotatef(err, "checking offer UUID for remote application %q", appName)
		} else if offerChanged {
			if err := w.runner.StopAndRemoveWorker(appName, w.catacomb.Dying()); err != nil && !errors.Is(err, errors.NotFound) {
				w.logger.Warningf(ctx, "error stopping remote application worker for %q: %v", appName, err)
			}
		}

		w.logger.Debugf(ctx, "starting watcher for remote application %q", appName)

		// Start the application worker to watch for things like new relations.
		if err := w.runner.StartWorker(ctx, appName, func(ctx context.Context) (worker.Worker, error) {
			return w.config.NewRemoteApplicationWorker(RemoteApplicationConfig{
				ApplicationName:            remoteApp.ApplicationName,
				LocalModelUUID:             w.config.ModelUUID,
				ConsumeVersion:             remoteApp.ConsumeVersion,
				RemoteRelationClientGetter: w.config.RemoteRelationClientGetter,
				Clock:                      w.config.Clock,
				Logger:                     w.logger,
			})
		}); err != nil && !errors.Is(err, errors.AlreadyExists) {
			return errors.Annotate(err, "error starting remote application worker")
		}
	}

	for _, name := range w.runner.WorkerNames() {
		if _, ok := witnessed[name]; ok {
			// We have witnessed this application, so we don't need to stop it.
			continue
		}

		w.logger.Debugf(ctx, "stopping remote application worker %q", name)
		if err := w.runner.StopAndRemoveWorker(name, w.catacomb.Dying()); err != nil && !errors.Is(err, errors.NotFound) {
			w.logger.Warningf(ctx, "error stopping remote application worker %q: %v", name, err)
		}
	}

	return nil
}

func (w *Worker) hasRemoteAppChanged(remoteApp crossmodelrelation.RemoteApplicationOfferrer) (bool, error) {
	appName := remoteApp.ApplicationName

	// If the worker for the name doesn't exist then that's ok, we just return
	// false to indicate that the offer UUID has not changed.
	worker, err := w.runner.Worker(remoteApp.ApplicationName, w.catacomb.Dying())
	if errors.Is(err, errors.NotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	// Now check if the remote application worker implements the
	// RemoteApplicationWorker interface, which provides the OfferUUID method.
	// If the offer UUID of consume version is different to the one we have,
	// then we need to stop the worker and start a new one.

	appWorker, ok := worker.(RemoteApplicationWorker)
	if !ok {
		return false, errors.Errorf("worker %q is not a RemoteApplicationWorker", appName)
	}

	return appWorker.OfferUUID() != remoteApp.OfferUUID ||
		appWorker.ConsumeVersion() != remoteApp.ConsumeVersion, nil
}

// Report provides information for the engine report.
func (w *Worker) Report() map[string]interface{} {
	return w.runner.Report()
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
