// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/relation"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

// ReportableWorker is an interface that allows a worker to be reported
// on by the engine.
type ReportableWorker interface {
	worker.Worker
	worker.Reporter
}

// RemoteApplicationWorker is an interface that defines the methods that a
// remote application worker must implement to be managed by the Worker.
type RemoteApplicationWorker interface {
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

// CrossModelService is an interface that groups together the local
// relation service and the cross-model relation service.
type CrossModelService interface {
	RelationService
	CrossModelRelationService
	StatusService
}

// RelationService is an interface that defines the methods for
// managing relations directly on the local model database.
type RelationService interface {
	// WatchApplicationLifeSuspendedStatus watches the changes to the
	// life suspended status of the specified application and notifies
	// the worker of any changes.
	WatchApplicationLifeSuspendedStatus(context.Context, application.UUID) (watcher.StringsWatcher, error)

	// GetRelationDetails returns RelationDetails for the given relationID.
	GetRelationDetails(context.Context, corerelation.UUID) (relation.RelationDetails, error)

	// WatchRelationUnits returns a watcher for changes to the units
	// in the given relation in the local model.
	WatchRelationUnits(context.Context, application.UUID) (watcher.NotifyWatcher, error)

	// GetRelationUnits returns the current state of the relation units.
	GetRelationUnits(context.Context, application.UUID) (relation.RelationUnitChange, error)
}

// CrossModelRelationService is an interface that defines the methods for
// managing cross-model relations directly on the local model database.
type CrossModelRelationService interface {
	// WatchRemoteApplicationOfferers watches the changes to remote
	// application consumers and notifies the worker of any changes.
	WatchRemoteApplicationOfferers(ctx context.Context) (watcher.NotifyWatcher, error)

	// GetRemoteApplicationOfferers returns the current state of all remote
	// application consumers in the local model.
	GetRemoteApplicationOfferers(context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error)

	// ConsumeRemoteRelationChange applies a relation change event received
	// from a remote model to the local model.
	ConsumeRemoteRelationChange(context.Context) error

	// ConsumeRemoteSecretChanges applies secret changes received
	// from a remote model to the local model.
	ConsumeRemoteSecretChanges(context.Context) error

	// ExportApplicationAndRelationToken exports the specified entities to the
	// remote model.
	ExportApplicationAndRelationToken(context.Context, names.Tag, names.Tag) (string, string, error)

	// SaveMacaroonForRelation saves the given macaroon for the specified remote
	// application.
	SaveMacaroonForRelation(context.Context, names.Tag, *macaroon.Macaroon) error

	// ImportRemoteApplicationToken imports a remote application token
	// into the local model.
	ImportRemoteApplicationToken(context.Context, names.Tag, string) error
}

// StatusService is an interface that defines the methods for
// managing status directly on the local model database.
type StatusService interface {
	// SetRemoteApplicationOffererStatus sets the status of the specified remote
	// application in the local model.
	SetRemoteApplicationOffererStatus(ctx context.Context, appName string, sts status.StatusInfo) error
}

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID                  model.UUID
	CrossModelService          CrossModelService
	RemoteRelationClientGetter RemoteRelationClientGetter
	NewRemoteApplicationWorker NewRemoteApplicationWorkerFunc

	NewLocalUnitRelationsWorker  NewLocalUnitRelationsWorkerFunc
	NewRemoteUnitRelationsWorker NewRemoteUnitRelationsWorkerFunc
	NewRemoteRelationsWorker     NewRemoteRelationsWorkerFunc

	Clock  clock.Clock
	Logger logger.Logger
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.CrossModelService == nil {
		return errors.NotValidf("nil CrossModelService")
	}
	if config.RemoteRelationClientGetter == nil {
		return errors.NotValidf("nil RemoteRelationClientGetter")
	}
	if config.NewRemoteApplicationWorker == nil {
		return errors.NotValidf("nil NewRemoteApplicationWorker")
	}
	if config.NewLocalUnitRelationsWorker == nil {
		return errors.NotValidf("nil NewLocalUnitRelationsWorker")
	}
	if config.NewRemoteUnitRelationsWorker == nil {
		return errors.NotValidf("nil NewRemoteUnitRelationsWorker")
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

	crossModelService CrossModelService

	logger logger.Logger
}

// New returns a Worker backed by config, or an error.
func NewWorker(config Config) (ReportableWorker, error) {
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

		crossModelService: config.CrossModelService,
		logger:            config.Logger,
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

// Kill the remote relation consumer worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait for the remote relation consumer worker to finish.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() (err error) {
	ctx := w.catacomb.Context(context.Background())

	// Watch for new remote application offerers. This is the consuming side,
	// so the consumer model has received an offer from another model.
	watcher, err := w.crossModelService.WatchRemoteApplicationOfferers(ctx)
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
	results, err := w.crossModelService.GetRemoteApplicationOfferers(ctx)
	if err != nil {
		return errors.Annotate(err, "querying remote applications")
	}

	witnessed := make(map[string]struct{})
	for _, remoteApp := range results {
		appName := remoteApp.ApplicationName
		appUUID := remoteApp.ApplicationUUID

		// We've witnessed the application, so we need to either start a new
		// worker or recreate it depending on if the offer has changed.
		witnessed[appUUID] = struct{}{}

		// Now check to see if the offer has changed for the remote application.
		if offerChanged, err := w.hasRemoteAppChanged(remoteApp); err != nil {
			return errors.Annotatef(err, "checking offer UUID for remote application %q", appName)
		} else if offerChanged {
			if err := w.runner.StopAndRemoveWorker(appUUID, w.catacomb.Dying()); err != nil && !errors.Is(err, errors.NotFound) {
				w.logger.Warningf(ctx, "error stopping remote application worker for %q: %v", appName, err)
			}
		}

		w.logger.Debugf(ctx, "starting watcher for remote application %q", appName)

		// Start the application worker to watch for things like new relations.
		if err := w.runner.StartWorker(ctx, appUUID, func(ctx context.Context) (worker.Worker, error) {
			return w.config.NewRemoteApplicationWorker(RemoteApplicationConfig{
				OfferUUID:                    remoteApp.OfferUUID,
				ApplicationName:              remoteApp.ApplicationName,
				ApplicationUUID:              application.UUID(remoteApp.ApplicationUUID),
				LocalModelUUID:               w.config.ModelUUID,
				RemoteModelUUID:              remoteApp.OffererModelUUID,
				ConsumeVersion:               remoteApp.ConsumeVersion,
				Macaroon:                     remoteApp.Macaroon,
				CrossModelService:            w.crossModelService,
				RemoteRelationClientGetter:   w.config.RemoteRelationClientGetter,
				NewLocalUnitRelationsWorker:  w.config.NewLocalUnitRelationsWorker,
				NewRemoteUnitRelationsWorker: w.config.NewRemoteUnitRelationsWorker,
				NewRemoteRelationsWorker:     w.config.NewRemoteRelationsWorker,
				Clock:                        w.config.Clock,
				Logger:                       w.logger,
			})
		}); err != nil && !errors.Is(err, errors.AlreadyExists) {
			return errors.Annotate(err, "error starting remote application worker")
		}
	}

	for _, appUUID := range w.runner.WorkerNames() {
		if _, ok := witnessed[appUUID]; ok {
			// We have witnessed this application, so we don't need to stop it.
			continue
		}

		w.logger.Debugf(ctx, "stopping remote application worker")
		if err := w.runner.StopAndRemoveWorker(appUUID, w.catacomb.Dying()); err != nil && !errors.Is(err, errors.NotFound) {
			w.logger.Warningf(ctx, "error stopping remote application worker %v", err)
		}
	}

	return nil
}

func (w *Worker) hasRemoteAppChanged(remoteApp crossmodelrelation.RemoteApplicationOfferer) (bool, error) {
	// If the worker for the remote application offerer doesn't exist then
	// that's ok, we just return false to indicate that the offer hasn't
	// changed.
	worker, err := w.runner.Worker(remoteApp.ApplicationUUID, w.catacomb.Dying())
	if errors.Is(err, errors.NotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	// Now check if the remote application offerer worker implements the
	// RemoteApplicationWorker interface, consume version is different to the
	// one we have, then we need to stop the worker and start a new one.

	appWorker, ok := worker.(RemoteApplicationWorker)
	if !ok {
		return false, errors.Errorf("worker %q is not a RemoteApplicationWorker", remoteApp.ApplicationName)
	}

	return appWorker.ConsumeVersion() != remoteApp.ConsumeVersion, nil
}

// Report provides information for the engine report.
func (w *Worker) Report() map[string]interface{} {
	return w.runner.Report()
}
