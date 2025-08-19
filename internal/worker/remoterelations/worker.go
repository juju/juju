// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"
	"fmt"
	"io"
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

// RemoteModelRelationsFacadeCloser implements RemoteModelRelationsFacade
// and add a Close() method.
type RemoteModelRelationsFacadeCloser interface {
	io.Closer
	RemoteModelRelationsFacade
}

// RemoteModelRelationsFacade instances publish local relation changes to the
// model hosting the remote application involved in the relation, and also watches
// for remote relation changes which are then pushed to the local model.
type RemoteModelRelationsFacade interface {
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

// CrossModelRelationService is a service that provides methods for
// managing cross-model relations and remote applications.
type CrossModelRelationService interface {
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

type newRemoteRelationsFacadeFunc func(context.Context, *api.Info) (RemoteModelRelationsFacadeCloser, error)

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID                 string
	CrossModelRelationService CrossModelRelationService
	NewRemoteModelFacadeFunc  newRemoteRelationsFacadeFunc
	Clock                     clock.Clock
	Logger                    logger.Logger
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.ModelUUID == "" {
		return errors.NotValidf("empty model uuid")
	}
	if config.CrossModelRelationService == nil {
		return errors.NotValidf("nil CrossModelRelationService")
	}
	if config.NewRemoteModelFacadeFunc == nil {
		return errors.NotValidf("nil Remote Model Facade func")
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
	config   Config
	logger   logger.Logger

	runner *worker.Runner

	service CrossModelRelationService

	// offerUUIDs records the offer UUID used for each saas name.
	offerUUIDs map[string]string
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
		IsFatal:       func(error) bool { return false },
		ShouldRestart: internalworker.ShouldRunnerRestart,

		// For any failures, try again in 15 seconds.
		RestartDelay: 15 * time.Second,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config:     config,
		offerUUIDs: make(map[string]string),
		runner:     runner,
		service:    config.CrossModelRelationService,
		logger:     config.Logger,
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
		ctx := w.scopedContext()
		w.logger.Errorf(ctx, "error in top level remote relations worker: %v", err)
	}
	return err
}

func (w *Worker) loop() (err error) {
	ctx := w.scopedContext()

	changes, err := w.service.WatchRemoteApplications(ctx)
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

		case applicationIds, ok := <-changes.Changes():
			if !ok {
				return errors.New("change channel closed")
			}

			if err := w.handleApplicationChanges(ctx, applicationIds); err != nil {
				return err
			}
		}
	}
}

func (w *Worker) handleApplicationChanges(ctx context.Context, applicationIds []string) error {
	if len(applicationIds) == 0 {
		return nil
	}

	w.logger.Debugf(ctx, "processing remote application changes for: %s", applicationIds)

	// Fetch the current state of each of the remote applications that have
	// changed.
	results, err := w.service.RemoteApplications(ctx, applicationIds)
	if err != nil {
		return errors.Annotate(err, "querying remote applications")
	}

	for i, result := range results {
		name := applicationIds[i]

		// The remote application may refer to an offer that has been removed
		// from the offering model, or it may refer to a new offer with a
		// different UUID. If it is for a new offer, we need to stop any current
		// worker for the old offer.
		appNotFound := result.Error != nil && params.IsCodeNotFound(result.Error)
		if result.Error != nil && !appNotFound {
			return errors.Annotatef(result.Error, "querying remote application %q", name)
		}

		var remoteApp *params.RemoteApplication
		var offerChanged bool
		if !appNotFound {
			remoteApp = result.Result

			existingOfferUUID, ok := w.offerUUIDs[remoteApp.Name]

			appNotFound = remoteApp.Status == string(status.Terminated) || remoteApp.Life == life.Dead
			offerChanged = ok && existingOfferUUID != remoteApp.OfferUUID
		}

		if appNotFound || offerChanged {
			// The remote application has been removed, stop its worker.
			w.logger.Debugf(ctx, "saas application %q gone from offering model", name)

			err := w.runner.StopAndRemoveWorker(name, w.catacomb.Dying())
			if err != nil && !errors.Is(err, errors.NotFound) {
				w.logger.Warningf(ctx, "error stopping saas worker for %q: %v", name, err)
			}

			delete(w.offerUUIDs, name)
			if appNotFound {
				continue
			}
		}

		startFunc := func(ctx context.Context) (worker.Worker, error) {
			appWorker := &remoteApplicationWorker{
				offerUUID:                         remoteApp.OfferUUID,
				applicationName:                   remoteApp.Name,
				localModelUUID:                    w.config.ModelUUID,
				remoteModelUUID:                   remoteApp.ModelUUID,
				isConsumerProxy:                   remoteApp.IsConsumerProxy,
				consumeVersion:                    remoteApp.ConsumeVersion,
				offerMacaroon:                     remoteApp.Macaroon,
				localRelationUnitChanges:          make(chan RelationUnitChangeEvent),
				remoteRelationUnitChanges:         make(chan RelationUnitChangeEvent),
				localModelFacade:                  w.service,
				newRemoteModelRelationsFacadeFunc: w.config.NewRemoteModelFacadeFunc,
				clock:                             w.config.Clock,
				logger:                            w.logger,
			}
			if err := catacomb.Invoke(catacomb.Plan{
				Name: "remote-application",
				Site: &appWorker.catacomb,
				Work: appWorker.loop,
			}); err != nil {
				return nil, errors.Trace(err)
			}
			return appWorker, nil
		}

		w.logger.Debugf(ctx, "starting watcher for remote application %q", name)

		// Start the application worker to watch for things like new relations.
		if err := w.runner.StartWorker(ctx, name, startFunc); errors.Is(err, errors.AlreadyExists) {
			w.logger.Debugf(ctx, "already running remote application worker for %q", name)
		} else if err != nil {
			return errors.Annotate(err, "error starting remote application worker")
		}

		w.offerUUIDs[name] = remoteApp.OfferUUID
	}
	return nil
}

// Report provides information for the engine report.
func (w *Worker) Report() map[string]interface{} {
	workers := make(map[string]interface{})
	for _, name := range w.runner.WorkerNames() {
		appWorker, err := w.runner.Worker(name, w.catacomb.Dying())
		if err != nil {
			workers[name] = fmt.Sprintf("ERROR: %v", err)
			continue
		}

		workers[name] = appWorker.(worker.Reporter).Report()
	}

	return map[string]interface{}{
		"workers": workers,
	}
}

func (w *Worker) scopedContext() context.Context {
	return w.catacomb.Context(context.Background())
}
