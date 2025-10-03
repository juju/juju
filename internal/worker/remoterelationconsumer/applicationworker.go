// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	"fmt"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/remoterelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/remoteunitrelations"
	"github.com/juju/juju/rpc/params"
)

// RemoteApplicationConfig defines the configuration for a remote application
// worker.
type RemoteApplicationConfig struct {
	CrossModelService          CrossModelService
	RemoteRelationClientGetter RemoteRelationClientGetter

	OfferUUID       string
	ApplicationName string
	ApplicationUUID application.UUID
	LocalModelUUID  model.UUID
	RemoteModelUUID string
	ConsumeVersion  int
	Macaroon        *macaroon.Macaroon

	NewLocalUnitRelationsWorker  NewLocalUnitRelationsWorkerFunc
	NewRemoteUnitRelationsWorker NewRemoteUnitRelationsWorkerFunc
	NewRemoteRelationsWorker     NewRemoteRelationsWorkerFunc

	Clock  clock.Clock
	Logger logger.Logger
}

// Validate ensures that the config is valid.
func (c RemoteApplicationConfig) Validate() error {
	if c.CrossModelService == nil {
		return errors.NotValidf("nil cross model service")
	}
	if c.RemoteRelationClientGetter == nil {
		return errors.NotValidf("nil remote relation client getter")
	}
	if c.OfferUUID == "" {
		return errors.NotValidf("empty offer uuid")
	}
	if c.ApplicationName == "" {
		return errors.NotValidf("empty application name")
	}
	if c.ApplicationUUID == "" {
		return errors.NotValidf("empty application uuid")
	}
	if c.LocalModelUUID == "" {
		return errors.NotValidf("empty local model uuid")
	}
	if c.RemoteModelUUID == "" {
		return errors.NotValidf("empty remote model uuid")
	}
	if c.Macaroon == nil {
		return errors.NotValidf("nil macaroon")
	}
	if c.NewLocalUnitRelationsWorker == nil {
		return errors.NotValidf("nil NewLocalUnitRelationsWorker")
	}
	if c.NewRemoteUnitRelationsWorker == nil {
		return errors.NotValidf("nil NewRemoteUnitRelationsWorker")
	}
	if c.NewRemoteRelationsWorker == nil {
		return errors.NotValidf("nil NewRemoteRelationsWorker")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil logger")
	}
	return nil
}

// remoteApplicationWorker listens for localChanges to relations
// involving a remote application, and publishes change to
// local relation units to the remote model. It also watches for
// changes originating from the offering model and consumes those
// in the local model.
type remoteApplicationWorker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner

	// crossModelService is the domain services used to interact with the local
	// database for cross model relations.
	crossModelService CrossModelService

	// remoteModelClient interacts with the remote (offering) model.
	remoteModelClient          RemoteModelRelationsClient
	remoteRelationClientGetter RemoteRelationClientGetter

	// These attributes are relevant to dealing with a specific
	// remote application proxy.
	offerUUID         string
	applicationName   string
	applicationUUID   application.UUID
	consumerModelUUID model.UUID
	offererModelUUID  string
	consumeVersion    int

	localRelationUnitChanges  chan relation.RelationUnitChange
	remoteRelationUnitChanges chan remoteunitrelations.RelationUnitChange
	remoteRelationChanges     chan remoterelations.RelationChange

	// offerMacaroon is used to confirm that permission has been granted to
	// consume the remote application to which this worker pertains.
	offerMacaroon *macaroon.Macaroon

	newLocalUnitRelationsWorker  NewLocalUnitRelationsWorkerFunc
	newRemoteUnitRelationsWorker NewRemoteUnitRelationsWorkerFunc
	newRemoteRelationsWorker     NewRemoteRelationsWorkerFunc

	clock  clock.Clock
	logger logger.Logger
}

// NewRemoteApplicationWorker creates a new remote application worker.
func NewRemoteApplicationWorker(config RemoteApplicationConfig) (ReportableWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "remote-application",
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: internalworker.ShouldRunnerRestart,
		Clock:         config.Clock,
		Logger:        internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &remoteApplicationWorker{
		runner:            runner,
		crossModelService: config.CrossModelService,

		localRelationUnitChanges:  make(chan relation.RelationUnitChange),
		remoteRelationUnitChanges: make(chan remoteunitrelations.RelationUnitChange),
		remoteRelationChanges:     make(chan remoterelations.RelationChange),

		offerUUID:         config.OfferUUID,
		applicationName:   config.ApplicationName,
		applicationUUID:   config.ApplicationUUID,
		consumerModelUUID: config.LocalModelUUID,
		offererModelUUID:  config.RemoteModelUUID,
		consumeVersion:    config.ConsumeVersion,
		offerMacaroon:     config.Macaroon,

		remoteRelationClientGetter: config.RemoteRelationClientGetter,

		newLocalUnitRelationsWorker:  config.NewLocalUnitRelationsWorker,
		newRemoteUnitRelationsWorker: config.NewRemoteUnitRelationsWorker,
		newRemoteRelationsWorker:     config.NewRemoteRelationsWorker,

		clock:  config.Clock,
		logger: config.Logger,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "remote-application",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{runner},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Kill is defined on worker.Worker
func (w *remoteApplicationWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *remoteApplicationWorker) Wait() error {
	err := w.catacomb.Wait()
	if err != nil {
		w.logger.Errorf(context.Background(), "error in remote application worker for %v: %v", w.applicationName, err)
	}
	return err
}

// ConsumeVersion returns the consume version for the remote application worker.
func (w *remoteApplicationWorker) ConsumeVersion() int {
	return w.consumeVersion
}

// Report provides information for the engine report.
func (w *remoteApplicationWorker) Report() map[string]interface{} {
	result := make(map[string]interface{})

	result["remote-model-uuid"] = w.offererModelUUID
	result["offer-uuid"] = w.offerUUID

	return result
}

func (w *remoteApplicationWorker) loop() (err error) {
	ctx := w.catacomb.Context(context.Background())

	// Watch for changes to any local relations to the remote application.
	relationsWatcher, err := w.crossModelService.WatchApplicationLifeSuspendedStatus(ctx, w.applicationUUID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "watching relations for remote application %q", w.applicationName)
	}
	if err := w.catacomb.Add(relationsWatcher); err != nil {
		return errors.Trace(err)
	}

	// Watch the offer changes on the offerer side, so that we can keep track
	// of the offer.

	w.remoteModelClient, err = w.setupRemoteModelClient(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	offerStatusWatcher, err := w.watchRemoteOfferStatus(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case changes, ok := <-relationsWatcher.Changes():
			if !ok {
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				default:
					return errors.New("relations watcher closed unexpectedly")
				}
			}
			w.logger.Debugf(ctx, "relations changed: %v", changes)

			for _, change := range changes {
				relationUUID := corerelation.UUID(change)

				// If we get an invalid UUID, log the error and continue, there
				// is nothing we can do about it, and we don't want the worker
				// to keep bouncing.
				if err := relationUUID.Validate(); err != nil {
					w.logger.Errorf(ctx, "invalid relation UUID %q: %v", change, err)
					continue
				}

				// Grab the relation details from the database, and handle the
				// change appropriately.
				details, err := w.crossModelService.GetRelationDetails(ctx, relationUUID)
				if errors.Is(err, relationerrors.RelationNotFound) {
					// Relation has been removed, ensure that we don't have
					// any workers still running for it.
					if err := w.handleRelationRemoved(ctx, relationUUID); err != nil {
						// If we fail to remove the relation, we must kill the
						// worker, as nothing will come around and try again.
						// Thus, kill it and force the application worker to
						// restart and start afresh.
						return errors.Annotatef(err, "cleaning up removed relation %q", relationUUID)
					}
				} else if err != nil {
					return errors.Annotate(err, "querying relations")
				}

				// The relation changed, we need to process the changed details.
				if err := w.handleRelationChange(ctx, details); err != nil {
					return errors.Annotatef(err, "handling change for relation %q", relationUUID)
				}
			}

		case changes, ok := <-offerStatusWatcher.Changes():
			if !ok {
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				default:
					return errors.New("offer status watcher closed unexpectedly")
				}
			}

			w.logger.Debugf(ctx, "offer status changed: %v", changes)

			for _, change := range changes {
				if err := w.crossModelService.SetRemoteApplicationOffererStatus(ctx, w.applicationName, change.Status); err != nil {
					return errors.Annotatef(err, "updating remote application %v status from remote model %v", w.applicationName, w.offererModelUUID)
				}

				// TODO (stickupkid): Handle terminated status.
			}
		}
	}
}

func (w *remoteApplicationWorker) setupRemoteModelClient(ctx context.Context) (RemoteModelRelationsClient, error) {
	remoteModelClient, err := w.remoteRelationClientGetter.GetRemoteRelationClient(ctx, w.offererModelUUID)
	if err == nil {
		return remoteModelClient, nil
	}

	// Attempt to set the status on the remote offer to indicate that
	// we cannot connect to the remote model. If this fails, log the error,
	// as we don't want the worker to bounce on this error. Instead, we want
	// to bounce on the original error.
	if err := w.crossModelService.SetRemoteApplicationOffererStatus(ctx, w.applicationName, status.StatusInfo{
		Status:  status.Error,
		Message: fmt.Sprintf("cannot connect to external controller: %v", err.Error()),
	}); err != nil {
		w.logger.Errorf(ctx, "failed updating remote application %v status from remote model %v: %v", w.applicationName, w.offererModelUUID, err)
	}
	return nil, errors.Annotate(err, "cannot connect to external controller")
}

func (w *remoteApplicationWorker) watchRemoteOfferStatus(ctx context.Context) (watcher.OfferStatusWatcher, error) {
	offerStatusWatcher, err := w.remoteModelClient.WatchOfferStatus(ctx, params.OfferArg{
		OfferUUID:     w.offerUUID,
		Macaroons:     macaroon.Slice{w.offerMacaroon},
		BakeryVersion: bakery.LatestVersion,
	})
	if isNotFound(err) {
		return nil, w.remoteOfferRemoved(ctx)
	} else if err != nil {
		if statusErr := w.setApplicationOffererStatusMacaroonError(ctx, err); statusErr != nil {
			w.logger.Errorf(ctx, "failed updating remote application %v status from remote model %v: %v", w.applicationName, w.offererModelUUID, statusErr)
		}
		return nil, errors.Annotate(err, "watching status for offer")
	}

	if err := w.catacomb.Add(offerStatusWatcher); err != nil {
		return nil, errors.Trace(err)
	}

	return offerStatusWatcher, nil
}

func (w *remoteApplicationWorker) setApplicationOffererStatusMacaroonError(ctx context.Context, err error) error {
	if params.ErrCode(err) != params.CodeDischargeRequired {
		return nil
	}

	if err := w.crossModelService.SetRemoteApplicationOffererStatus(ctx, w.applicationName, status.StatusInfo{
		Status:  status.Error,
		Message: err.Error(),
	}); err != nil {
		return err
	}
	return nil
}

func (w *remoteApplicationWorker) remoteOfferRemoved(ctx context.Context) error {
	w.logger.Debugf(ctx, "remote offer for %s has been removed", w.applicationName)

	// TODO (stickupkid): What's the point of setting the status to terminated,
	// and not removing any additional workers? If the offer has been removed,
	// surely we want to some how trigger a cleanup of the remote application
	// and data associated with it?
	if err := w.crossModelService.SetRemoteApplicationOffererStatus(ctx, w.applicationName, status.StatusInfo{
		Status:  status.Terminated,
		Message: "offer has been removed",
	}); err != nil {
		return errors.Annotatef(err, "updating remote application %v status from remote model %v", w.applicationName, w.offererModelUUID)
	}
	return nil
}

func (w *remoteApplicationWorker) handleRelationRemoved(ctx context.Context, relationUUID corerelation.UUID) error {
	w.logger.Debugf(ctx, "relation %q removed", relationUUID)
	return errors.NotImplemented
}

// relationChanged processes changes to the relation as recorded in the
// local model when a change event arrives from the remote model.
func (w *remoteApplicationWorker) handleRelationChange(ctx context.Context, details relation.RelationDetails) error {
	w.logger.Debugf(ctx, "relation %q changed in local model: %v", details.UUID, details)

	switch {
	case details.Life != life.Alive:
		// TODO (stickupkid): Handle the case where the relation is dying,
		// but there are still units in scope. We need to ensure that we
		// don't kill the relation until all units have departed.
		w.logger.Debugf(ctx, "relation %v is not alive (%v)", details.UUID, details.Life)
		return errors.NotImplementedf("handling non-alive relation changes")
	case details.Suspended:
		return w.handleRelationSuspended(ctx, details)
	default:
		return w.handleRelationConsumption(ctx, details)
	}
}

func (w *remoteApplicationWorker) handleRelationSuspended(ctx context.Context, details relation.RelationDetails) error {
	w.logger.Debugf(ctx, "(%v) relation %v suspended", details.Life, details.UUID)
	return nil
}

// handleRelationConsumption handles the case where a relation is alive and not
// suspended, meaning that we should ensure that it is registered on the
// offering side.
func (w *remoteApplicationWorker) handleRelationConsumption(
	ctx context.Context,
	details relation.RelationDetails,
) error {
	// Relation key is derived from the endpoint identifiers.
	var synthEndpoint relation.Endpoint
	var otherEndpointName string
	for _, e := range details.Endpoints {
		if e.ApplicationName == w.applicationName {
			synthEndpoint = e
		} else {
			otherEndpointName = e.Name
		}
	}

	// If the relation does not have an endpoint for either the local
	// application or the remote application, then we cannot proceed.
	if otherEndpointName == "" || synthEndpoint.Name == "" {
		return errors.NotValidf("relation %v does not have endpoints for local application %q and remote application", details.UUID, w.applicationName)
	}

	// We have not seen the relation before, or the relation was suspended, make
	// sure it is registered (ack'd) on the offering side.
	result, err := w.registerConsumerRelation(
		ctx,
		details.UUID, w.offerUUID, w.consumeVersion,
		synthEndpoint, otherEndpointName,
	)
	if err != nil {
		if statusErr := w.setApplicationOffererStatusMacaroonError(ctx, err); statusErr != nil {
			w.logger.Errorf(ctx, "failed updating remote application %v status from remote model %v: %v", w.applicationName, w.offererModelUUID, statusErr)
		}
		return errors.Annotatef(err, "registering application %q and relation %q", w.applicationName, details.UUID)
	}

	w.logger.Debugf(ctx, "consumer relation registered for %q: %v", details.UUID, result)

	// Start the remote relation worker to watch for offerer relation changes.
	// The aim is to ensure that we can track the suspended status of the
	// relation so we can correctly react to that.
	if err := w.createOffererRelationWorker(ctx, details.UUID, result.macaroon); err != nil {
		return errors.Annotatef(err, "creating offerer relation worker for %q", details.UUID)
	}

	return nil
}

// Create a new worker to watch for changes to the relation in the offering
// model.
func (w *remoteApplicationWorker) createOffererRelationWorker(ctx context.Context, relationUUID corerelation.UUID, mac *macaroon.Macaroon) error {
	if err := w.runner.StartWorker(ctx, relationUUID.String(), func(ctx context.Context) (worker.Worker, error) {
		return w.newRemoteRelationsWorker(remoterelations.Config{
			Client:   w.remoteModelClient,
			Macaroon: mac,
			Changes:  w.remoteRelationChanges,
			Clock:    w.clock,
			Logger:   w.logger,
		})

	}); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return errors.Annotatef(err, "starting offerer relation worker for %q", relationUUID)
	}

	return nil
}

// registerConsumerRelation registers the relation in the offering model,
// and returns the offering application UUID and macaroon for the relation.
// The relation has been created in the consumer model.
func (w *remoteApplicationWorker) registerConsumerRelation(
	ctx context.Context,
	relationUUID corerelation.UUID, offerUUID string, consumeVersion int,
	applicationEndpointIdent relation.Endpoint, remoteEndpointName string,
) (consumerRelationResult, error) {
	w.logger.Debugf(ctx, "register consumer relation %q to synthetic offerer application %q", relationUUID, w.applicationName)

	// This data goes to the remote model so we map local info
	// from this model to the remote arg values and visa versa.
	arg := params.RegisterRemoteRelationArg{
		ApplicationToken: w.applicationUUID.String(),
		SourceModelTag:   names.NewModelTag(w.consumerModelUUID.String()).String(),
		RelationToken:    relationUUID.String(),
		OfferUUID:        offerUUID,
		RemoteEndpoint: params.RemoteEndpoint{
			Name:      applicationEndpointIdent.Name,
			Role:      applicationEndpointIdent.Role,
			Interface: applicationEndpointIdent.Interface,
			Limit:     applicationEndpointIdent.Limit,
		},
		LocalEndpointName: remoteEndpointName,
		ConsumeVersion:    consumeVersion,
		Macaroons:         macaroon.Slice{w.offerMacaroon},
		BakeryVersion:     bakery.LatestVersion,
	}
	offererRelations, err := w.remoteModelClient.RegisterRemoteRelations(ctx, arg)
	if err != nil {
		return consumerRelationResult{}, errors.Trace(err)
	} else if len(offererRelations) == 0 {
		return consumerRelationResult{}, errors.New("no result from registering remote relation")
	} else if len(offererRelations) > 1 {
		w.logger.Infof(ctx, "expected one result from registering remote relation, got %d", len(offererRelations))
	}

	// We've guarded against this from being out of bounds, so it's safe to do
	// a raw access.
	offererRelation := offererRelations[0]
	if err := offererRelation.Error; err != nil {
		return consumerRelationResult{}, errors.Annotatef(err, "registering relation %q", relationUUID)
	}

	// Import the application UUID from the offering model.
	registerResult := *offererRelation.Result

	// The register result token is always a UUID. This is the case for 3.x
	// and onwards. This should ensure that we're backwards compatible with
	// anything that the remote model is running.
	//
	// See: https://github.com/juju/juju/blob/43e381811d9e330ee2d095c1e0562300bd78b68a/state/remoteentities.go#L110-L115
	offeringAppUUID, err := application.ParseID(registerResult.Token)
	if err != nil {
		return consumerRelationResult{}, errors.Annotatef(err, "parsing offering application token %q", registerResult.Token)
	}

	// We have a new macaroon attenuated to the relation.
	// Save for the firewaller.
	if err := w.crossModelService.SaveMacaroonForRelation(ctx, relationUUID, registerResult.Macaroon); err != nil {
		return consumerRelationResult{}, errors.Annotatef(err, "saving macaroon for %q", relationUUID)
	}

	return consumerRelationResult{
		offeringApplicationUUID: offeringAppUUID,
		macaroon:                registerResult.Macaroon,
	}, nil
}

type consumerRelationResult struct {
	offeringApplicationUUID application.UUID
	macaroon                *macaroon.Macaroon
}

// isNotFound allows either type of not found error
// to be correctly handled.
// TODO(wallyworld) - remove when all api facades are fixed
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errors.NotFound) || params.IsCodeNotFound(err)
}
