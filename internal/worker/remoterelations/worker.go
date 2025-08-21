// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

// RemoteApplicationWorker is an interface that defines the methods that a
// remote application worker must implement to be managed by the Worker.
type RemoteApplicationWorker interface {
	// ApplicationID returns the application ID for the remote application
	// worker.
	ApplicationID() string

	// ApplicationName returns the application name for the remote application
	// worker.
	ApplicationName() string

	// OfferUUID returns the offer UUID for the remote application worker.
	OfferUUID() string
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

// RemoteRelationsFacade exposes remote relation functionality to a worker.
// This is the local model's view of the remote relations API.
type RemoteRelationsFacade interface {
	// ImportRemoteEntity adds an entity to the remote entities collection
	// with the specified opaque token.
	ImportRemoteEntity(ctx context.Context, entity names.Tag, token string) error

	// SaveMacaroon saves the macaroon for the entity.
	SaveMacaroon(ctx context.Context, entity names.Tag, mac *macaroon.Macaroon) error

	// ExportEntities allocates unique, remote entity IDs for the
	// given entities in the local model.
	ExportEntities(context.Context, []names.Tag) ([]params.TokenResult, error)

	// GetToken returns the token associated with the entity with the given tag.
	GetToken(context.Context, names.Tag) (string, error)

	// Relations returns information about the relations
	// with the specified keys in the local model.
	Relations(ctx context.Context, keys []string) ([]params.RemoteRelationResult, error)

	// RemoteApplications returns the current state of the remote applications with
	// the specified names in the local model.
	RemoteApplications(ctx context.Context, names []string) ([]params.RemoteApplicationResult, error)

	// WatchLocalRelationChanges returns a watcher that notifies of changes to the
	// local units in the relation with the given key.
	WatchLocalRelationChanges(ctx context.Context, relationKey string) (apiwatcher.RemoteRelationWatcher, error)

	// WatchRemoteApplications watches for addition, removal and lifecycle
	// changes to remote applications known to the local model.
	WatchRemoteApplications(ctx context.Context) (watcher.StringsWatcher, error)

	// WatchRemoteApplicationRelations starts a StringsWatcher for watching the relations of
	// each specified application in the local model, and returns the watcher IDs
	// and initial values, or an error if the application's relations could not be
	// watched.
	WatchRemoteApplicationRelations(ctx context.Context, application string) (watcher.StringsWatcher, error)

	// ConsumeRemoteRelationChange consumes a change to settings originating
	// from the remote/offering side of a relation.
	ConsumeRemoteRelationChange(ctx context.Context, change params.RemoteRelationChangeEvent) error

	// ControllerAPIInfoForModel returns the controller api info for a model.
	ControllerAPIInfoForModel(ctx context.Context, modelUUID string) (*api.Info, error)

	// SetRemoteApplicationStatus sets the status for the specified remote application.
	SetRemoteApplicationStatus(ctx context.Context, applicationName string, status status.Status, message string) error

	// UpdateControllerForModel ensures that there is an external controller record
	// for the input info, associated with the input model ID.
	UpdateControllerForModel(ctx context.Context, controller crossmodel.ControllerInfo, modelUUID string) error

	// ConsumeRemoteSecretChanges updates the local model with secret revision  changes
	// originating from the remote/offering model.
	ConsumeRemoteSecretChanges(ctx context.Context, changes []watcher.SecretRevisionChange) error
}

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID                  string
	RelationsFacade            RemoteRelationsFacade
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
	if config.RelationsFacade == nil {
		return errors.NotValidf("nil Facade")
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
		logger: config.Logger,
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

	changes, err := w.config.RelationsFacade.WatchRemoteApplications(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(changes); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case applicationIDs, ok := <-changes.Changes():
			if !ok {
				return errors.New("change channel closed")
			}

			if err := w.handleApplicationChanges(ctx, applicationIDs); err != nil {
				return err
			}
		}
	}
}

func (w *Worker) handleApplicationChanges(ctx context.Context, applicationIDs []string) error {
	w.logger.Debugf(ctx, "processing remote application changes for: %s", applicationIDs)

	// Fetch the current state of each of the remote applications that have changed.
	results, err := w.config.RelationsFacade.RemoteApplications(ctx, applicationIDs)
	if err != nil {
		return errors.Annotate(err, "querying remote applications")
	}

	for i, result := range results {
		applicationID := applicationIDs[i]

		// The remote application may refer to an offer that has been removed
		// from the offering model, or it may refer to a new offer with a
		// different UUID. If it is for a new offer, we need to stop any current
		// worker for the old offer.
		notFound := result.Error != nil && params.IsCodeNotFound(result.Error)
		if result.Error != nil && !notFound {
			return errors.Annotatef(result.Error, "querying remote applications")
		} else if notFound || appTerminated(result.Result) {
			// The remote application has been removed, stop its worker.
			appName, err := w.getApplicationNameFromID(applicationID)
			if err != nil && !errors.Is(err, errors.NotFound) {
				return errors.Annotatef(err, "getting application name for ID %q", applicationID)
			}
			if err := w.runner.StopAndRemoveWorker(appName, w.catacomb.Dying()); err != nil && !errors.Is(err, errors.NotFound) {
				w.logger.Warningf(ctx, "error stopping remote application worker for %q: %v", appName, err)
			}

			// We don't need to do anything else for this application, so
			// continue to the next one.
			continue
		}

		remoteApp := result.Result
		appName := remoteApp.Name

		// Now check to see if the offer UUID has changed for the remote
		// application.
		offerChanged, err := w.hasRemoteAppChanged(appName, result.Result.OfferUUID)
		if err != nil {
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
				OfferUUID:                  remoteApp.OfferUUID,
				ApplicationID:              applicationID,
				ApplicationName:            remoteApp.Name,
				LocalModelUUID:             w.config.ModelUUID,
				RemoteModelUUID:            remoteApp.ModelUUID,
				IsConsumerProxy:            remoteApp.IsConsumerProxy,
				ConsumeVersion:             remoteApp.ConsumeVersion,
				Macaroon:                   remoteApp.Macaroon,
				RemoteRelationsFacade:      w.config.RelationsFacade,
				RemoteRelationClientGetter: w.config.RemoteRelationClientGetter,
				Clock:                      w.config.Clock,
				Logger:                     w.logger,
			})
		}); err != nil && !errors.Is(err, errors.AlreadyExists) {
			return errors.Annotate(err, "error starting remote application worker")
		}
	}
	return nil
}

func (w *Worker) getApplicationNameFromID(applicationID string) (string, error) {
	for _, name := range w.runner.WorkerNames() {
		worker, err := w.runner.Worker(name, w.catacomb.Dying())
		if errors.Is(err, errors.NotFound) {
			continue
		} else if err != nil {
			return "", err
		}

		r, ok := worker.(RemoteApplicationWorker)
		if !ok {
			return "", errors.Errorf("worker %q is not a RemoteApplicationWorker", worker)
		}

		if r.ApplicationID() == applicationID {
			return r.ApplicationName(), nil
		}
	}

	return "", errors.NotFoundf("remote application with ID %q", applicationID)
}

func (w *Worker) hasRemoteAppChanged(name, offerUUID string) (bool, error) {
	// If the worker for the name doesn't exist then that's ok, we just return
	// false to indicate that the offer UUID has not changed.
	remoteApp, err := w.runner.Worker(name, w.catacomb.Dying())
	if errors.Is(err, errors.NotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	// Now check if the remote application worker implements the
	// RemoteApplicationWorker interface, which provides the OfferUUID method.
	// If the offer UUID is different to the one we have, then we need to
	// stop the worker and start a new one.

	appWorker, ok := remoteApp.(RemoteApplicationWorker)
	if !ok {
		return false, errors.Errorf("worker %q is not a RemoteApplicationWorker", name)
	}

	return appWorker.OfferUUID() != offerUUID, nil
}

func appTerminated(remoteApp *params.RemoteApplication) bool {
	return remoteApp.Status == string(status.Terminated) || remoteApp.Life == life.Dead
}

// Report provides information for the engine report.
func (w *Worker) Report() map[string]interface{} {
	return w.runner.Report()
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
