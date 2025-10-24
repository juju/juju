// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/domain/removal"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

const (
	// RemoteApplicationOffererDeadErr is returned when the remote application
	// offerer is no longer required because it has been removed. This prevents
	// the worker from being restarted.
	RemoteApplicationOffererDeadErr = errors.ConstError("remote application offerer is dead and no longer required")
)

// ReportableWorker is an interface that allows a worker to be reported
// on by the engine.
type ReportableWorker interface {
	worker.Worker
	worker.Reporter
}

// OffererApplicationWorker is an interface that defines the methods that a
// remote application worker must implement to be managed by the Worker.
type OffererApplicationWorker interface {
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
	// given remote token.
	WatchRelationChanges(_ context.Context, relationToken string, macs macaroon.Slice) (apiwatcher.RemoteRelationWatcher, error)

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
	RemovalService
}

// RelationService is an interface that defines the methods for
// managing relations directly on the local model database.
type RelationService interface {
	// WatchRelationsLifeSuspendedStatusForApplication watches the changes to the
	// life suspended status of the specified application and notifies
	// the worker of any changes.
	WatchRelationsLifeSuspendedStatusForApplication(context.Context, application.UUID) (watcher.StringsWatcher, error)

	// GetRelationDetails returns RelationDetails for the given relationID.
	GetRelationDetails(context.Context, corerelation.UUID) (relation.RelationDetails, error)

	// WatchRelationUnits returns a watcher for changes to the units
	// in the given relation in the local model.
	WatchRelationUnits(context.Context, corerelation.UUID, application.UUID) (watcher.NotifyWatcher, error)

	// GetRelationUnits returns the current state of the relation units.
	GetRelationUnits(context.Context, corerelation.UUID, application.UUID) (relation.RelationUnitChange, error)

	// GetRelationUnitUUID returns the relation unit UUID for the given unit for
	// the given relation.
	GetRelationUnitUUID(
		ctx context.Context,
		relationUUID corerelation.UUID,
		unitName unit.Name,
	) (corerelation.UnitUUID, error)

	// SetRelationRemoteApplicationAndUnitSettings will set the application and
	// unit settings for a remote relation. If the unit has not yet entered
	// scope, it will force the unit to enter scope. All settings will be
	// replaced with the provided settings.
	// This will ensure that the application, relation and units exist and that
	// they are alive.
	SetRelationRemoteApplicationAndUnitSettings(
		ctx context.Context,
		applicationUUID application.UUID,
		relationUUID corerelation.UUID,
		applicationSettings map[string]string,
		unitSettings map[unit.Name]map[string]string,
	) error

	// SetRemoteRelationSuspendedState sets the suspended state of the specified
	// remote relation in the local model.
	SetRemoteRelationSuspendedState(ctx context.Context, relationUUID corerelation.UUID, suspended bool, reason string) error
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

	// ConsumeRemoteSecretChanges applies secret changes received
	// from a remote model to the local model.
	ConsumeRemoteSecretChanges(context.Context) error

	// SaveMacaroonForRelation saves the given macaroon for the specified remote
	// application.
	SaveMacaroonForRelation(context.Context, corerelation.UUID, *macaroon.Macaroon) error

	// EnsureUnitsExist ensures that the specified units exist in the local
	// model, creating any that are missing.
	EnsureUnitsExist(ctx context.Context, appUUID application.UUID, units []unit.Name) error
}

// StatusService is an interface that defines the methods for
// managing status directly on the local model database.
type StatusService interface {
	// SetRemoteApplicationOffererStatus sets the status of the specified remote
	// application in the local model.
	SetRemoteApplicationOffererStatus(ctx context.Context, appName string, sts status.StatusInfo) error
}

// RemovalService is an interface that defines the methods for
// removing relations directly on the local model database.
type RemovalService interface {
	// RemoveRelation sets the relation with the given relation UUID
	// from the local model to dying.
	RemoveRemoteRelation(
		ctx context.Context, relUUID corerelation.UUID, force bool, wait time.Duration,
	) (removal.UUID, error)

	// RemoveRemoteApplicationOffererByApplicationUUID sets the remote
	// application offerer with the given application UUID from the local model
	// to dying.
	RemoveRemoteApplicationOffererByApplicationUUID(
		ctx context.Context, appUUID application.UUID, force bool, duration time.Duration) (removal.UUID, error)

	// LeaveScope updates the relation to indicate that the unit represented by
	// the input relation unit UUID is not in the implied relation scope.
	LeaveScope(ctx context.Context, relationUnitUUID corerelation.UnitUUID) error
}

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID                  model.UUID
	CrossModelService          CrossModelService
	RemoteRelationClientGetter RemoteRelationClientGetter
	NewLocalConsumerWorker     NewLocalConsumerWorkerFunc

	NewConsumerUnitRelationsWorker NewConsumerUnitRelationsWorkerFunc
	NewOffererUnitRelationsWorker  NewOffererUnitRelationsWorkerFunc
	NewOffererRelationsWorker      NewOffererRelationsWorkerFunc

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
	if config.NewLocalConsumerWorker == nil {
		return errors.NotValidf("nil NewLocalConsumerWorker")
	}
	if config.NewConsumerUnitRelationsWorker == nil {
		return errors.NotValidf("nil NewConsumerUnitRelationsWorker")
	}
	if config.NewOffererUnitRelationsWorker == nil {
		return errors.NotValidf("nil NewOffererUnitRelationsWorker")
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

		// Only restart if the worker has not indicated that it should not
		// be restarted.
		ShouldRestart: func(err error) bool {
			if internalworker.ShouldRunnerRestart(err) {
				return true
			}
			return !errors.Is(err, RemoteApplicationOffererDeadErr)
		},

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
	w.logger.Debugf(ctx, "processing offerer application worker changes")

	// Fetch the current state of each of the offerer application workers that
	// have changed.
	results, err := w.crossModelService.GetRemoteApplicationOfferers(ctx)
	if err != nil {
		return errors.Annotate(err, "querying offerer application workers")
	}

	witnessed := make(map[string]struct{})
	for _, remoteApp := range results {
		appName := remoteApp.ApplicationName
		appUUID := remoteApp.ApplicationUUID

		// We've witnessed the application, so we need to either start a new
		// worker or recreate it depending on if the offer has changed.
		witnessed[appUUID] = struct{}{}

		// Now check to see if the offer has changed for the offerer application
		// worker.
		if offerChanged, err := w.hasRemoteAppChanged(remoteApp); err != nil {
			return errors.Annotatef(err, "checking offer UUID for offerer application worker %q", appName)
		} else if offerChanged {
			if err := w.runner.StopAndRemoveWorker(appUUID, w.catacomb.Dying()); err != nil && !errors.Is(err, errors.NotFound) {
				w.logger.Warningf(ctx, "error stopping offerer application worker worker for %q: %v", appName, err)
			}
		}

		w.logger.Debugf(ctx, "starting watcher for offerer application worker %q", appName)

		// Start the application worker to watch for things like new relations.
		if err := w.runner.StartWorker(ctx, appUUID, func(ctx context.Context) (worker.Worker, error) {
			return w.config.NewLocalConsumerWorker(LocalConsumerWorkerConfig{
				OfferUUID:                      remoteApp.OfferUUID,
				ApplicationName:                remoteApp.ApplicationName,
				ApplicationUUID:                application.UUID(remoteApp.ApplicationUUID),
				ConsumerModelUUID:              w.config.ModelUUID,
				OffererModelUUID:               remoteApp.OffererModelUUID,
				ConsumeVersion:                 remoteApp.ConsumeVersion,
				Macaroon:                       remoteApp.Macaroon,
				CrossModelService:              w.crossModelService,
				RemoteRelationClientGetter:     w.config.RemoteRelationClientGetter,
				NewConsumerUnitRelationsWorker: w.config.NewConsumerUnitRelationsWorker,
				NewOffererUnitRelationsWorker:  w.config.NewOffererUnitRelationsWorker,
				NewOffererRelationsWorker:      w.config.NewOffererRelationsWorker,
				Clock:                          w.config.Clock,
				Logger:                         w.logger,
			})
		}); err != nil && !errors.Is(err, errors.AlreadyExists) {
			return errors.Annotate(err, "error starting offerer application worker worker")
		}
	}

	for _, appUUID := range w.runner.WorkerNames() {
		if _, ok := witnessed[appUUID]; ok {
			// We have witnessed this application, so we don't need to stop it.
			continue
		}

		w.logger.Debugf(ctx, "stopping offerer application worker worker")
		if err := w.runner.StopAndRemoveWorker(appUUID, w.catacomb.Dying()); err != nil && !errors.Is(err, errors.NotFound) {
			w.logger.Warningf(ctx, "error stopping offerer application worker worker %v", err)
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
	// OffererApplicationWorker interface, consume version is different to the
	// one we have, then we need to stop the worker and start a new one.

	appWorker, ok := worker.(OffererApplicationWorker)
	if !ok {
		return false, errors.Errorf("worker %q is not a OffererApplicationWorker", remoteApp.ApplicationName)
	}

	return appWorker.ConsumeVersion() != remoteApp.ConsumeVersion, nil
}

// Report provides information for the engine report.
func (w *Worker) Report() map[string]interface{} {
	return w.runner.Report()
}
