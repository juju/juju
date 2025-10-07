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
	internalerrors "github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/consumerunitrelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/offererrelations"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/offererunitrelations"
	"github.com/juju/juju/rpc/params"
)

const (
	// ErrPermissionRevokedWhilstDying identifies when permission is revoked
	// whilst the relation is in a dying state. This is a terminal error and
	// requires the worker to be restarted to recover.
	ErrPermissionRevokedWhilstDying = internalerrors.ConstError("relation permission revoked whilst dying")
)

// LocalConsumerWorkerConfig defines the configuration for a local consumer
// worker.
type LocalConsumerWorkerConfig struct {
	CrossModelService          CrossModelService
	RemoteRelationClientGetter RemoteRelationClientGetter

	OfferUUID         string
	ApplicationName   string
	ApplicationUUID   application.UUID
	ConsumerModelUUID model.UUID
	OffererModelUUID  string
	ConsumeVersion    int
	Macaroon          *macaroon.Macaroon

	NewConsumerUnitRelationsWorker NewConsumerUnitRelationsWorkerFunc
	NewOffererUnitRelationsWorker  NewOffererUnitRelationsWorkerFunc
	NewOffererRelationsWorker      NewOffererRelationsWorkerFunc

	Clock  clock.Clock
	Logger logger.Logger
}

// Validate ensures that the config is valid.
func (c LocalConsumerWorkerConfig) Validate() error {
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
	if c.ConsumerModelUUID == "" {
		return errors.NotValidf("empty consumer model uuid")
	}
	if c.OffererModelUUID == "" {
		return errors.NotValidf("empty offerer model uuid")
	}
	if c.Macaroon == nil {
		return errors.NotValidf("nil macaroon")
	}
	if c.NewConsumerUnitRelationsWorker == nil {
		return errors.NotValidf("nil NewConsumerUnitRelationsWorker")
	}
	if c.NewOffererUnitRelationsWorker == nil {
		return errors.NotValidf("nil NewOffererUnitRelationsWorker")
	}
	if c.NewOffererRelationsWorker == nil {
		return errors.NotValidf("nil NewOffererRelationsWorker")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil logger")
	}
	return nil
}

// localConsumerWorker listens for consumer changes to relations involving a
// offerer application, and publishes change to consumer relation units to the
// offerer model. It also watches for changes originating from the offering
// model and consumes those in the consumer model.
type localConsumerWorker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner

	// crossModelService is the domain services used to interact with the
	// consumer's database for cross model relations.
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

	// offerMacaroon is used to confirm that permission has been granted to
	// consume the remote application to which this worker pertains.
	offerMacaroon *macaroon.Macaroon

	consumerRelationUnitChanges chan relation.RelationUnitChange
	offererRelationUnitChanges  chan offererunitrelations.RelationUnitChange
	offererRelationChanges      chan offererrelations.RelationChange

	newConsumerUnitRelationsWorker NewConsumerUnitRelationsWorkerFunc
	newOffererUnitRelationsWorker  NewOffererUnitRelationsWorkerFunc
	newOffererRelationsWorker      NewOffererRelationsWorkerFunc

	clock  clock.Clock
	logger logger.Logger
}

// NewLocalConsumerWorker creates a new local consumer worker.
func NewLocalConsumerWorker(config LocalConsumerWorkerConfig) (ReportableWorker, error) {
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

	w := &localConsumerWorker{
		runner:            runner,
		crossModelService: config.CrossModelService,

		offerUUID:         config.OfferUUID,
		applicationName:   config.ApplicationName,
		applicationUUID:   config.ApplicationUUID,
		consumerModelUUID: config.ConsumerModelUUID,
		offererModelUUID:  config.OffererModelUUID,
		consumeVersion:    config.ConsumeVersion,
		offerMacaroon:     config.Macaroon,

		remoteRelationClientGetter: config.RemoteRelationClientGetter,

		consumerRelationUnitChanges: make(chan relation.RelationUnitChange),
		offererRelationUnitChanges:  make(chan offererunitrelations.RelationUnitChange),
		offererRelationChanges:      make(chan offererrelations.RelationChange),

		newConsumerUnitRelationsWorker: config.NewConsumerUnitRelationsWorker,
		newOffererUnitRelationsWorker:  config.NewOffererUnitRelationsWorker,
		newOffererRelationsWorker:      config.NewOffererRelationsWorker,

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
func (w *localConsumerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *localConsumerWorker) Wait() error {
	err := w.catacomb.Wait()
	if err != nil {
		w.logger.Errorf(context.Background(), "error in remote application worker for %v: %v", w.applicationName, err)
	}
	return err
}

// ConsumeVersion returns the consume version for the remote application worker.
func (w *localConsumerWorker) ConsumeVersion() int {
	return w.consumeVersion
}

// Report provides information for the engine report.
func (w *localConsumerWorker) Report() map[string]interface{} {
	result := make(map[string]interface{})

	result["remote-model-uuid"] = w.offererModelUUID
	result["offer-uuid"] = w.offerUUID
	result["application-name"] = w.applicationName
	result["application-uuid"] = w.applicationUUID
	result["consumer-model-uuid"] = w.consumerModelUUID
	result["consume-version"] = w.consumeVersion

	result["workers"] = w.runner.Report()

	return result
}

func (w *localConsumerWorker) loop() (err error) {
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

func (w *localConsumerWorker) setupRemoteModelClient(ctx context.Context) (RemoteModelRelationsClient, error) {
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

func (w *localConsumerWorker) watchRemoteOfferStatus(ctx context.Context) (watcher.OfferStatusWatcher, error) {
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

func (w *localConsumerWorker) setApplicationOffererStatusMacaroonError(ctx context.Context, err error) error {
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

func (w *localConsumerWorker) remoteOfferRemoved(ctx context.Context) error {
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

func (w *localConsumerWorker) handleRelationRemoved(ctx context.Context, relationUUID corerelation.UUID) error {
	w.logger.Debugf(ctx, "relation %q removed", relationUUID)
	return errors.NotImplemented
}

// relationChanged processes changes to the relation as recorded in the
// local model when a change event arrives from the remote model.
func (w *localConsumerWorker) handleRelationChange(ctx context.Context, details relation.RelationDetails) error {
	w.logger.Debugf(ctx, "relation %q changed in local model: %v", details.UUID, details)

	switch {
	case details.Suspended:
		return w.handleRelationSuspended(ctx, details)
	default:
		return w.handleRelationConsumption(ctx, details)
	}
}

func (w *localConsumerWorker) handleRelationSuspended(ctx context.Context, details relation.RelationDetails) error {
	w.logger.Debugf(ctx, "(%v) relation %v suspended", details.Life, details.UUID)
	return nil
}

// handleRelationConsumption handles the case where a relation is alive and not
// suspended, meaning that we should ensure that it is registered on the
// offering side.
func (w *localConsumerWorker) handleRelationConsumption(
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
	if err := w.ensureOffererRelationWorker(ctx, details.UUID, result.offererApplicationUUID, result.macaroon); err != nil {
		return errors.Annotatef(err, "creating offerer relation worker for %q", details.UUID)
	}

	// Create the unit watchers for both the consumer and offerer sides if the
	// relation is not suspended. It is expected that the unit watchers will
	// clean themselves up if the relation is suspended or removed.
	if !details.Suspended {
		if err := w.ensureUnitRelationWorkers(ctx, details, result.offererApplicationUUID, result.macaroon); err != nil {
			return errors.Annotatef(err, "creating unit relation workers for %q", details.UUID)
		}
	}

	// Handle the case where the relation is dying, and ensure we have no
	// workers still running for it.
	if details.Life != life.Alive {
		return w.handleRelationDying(ctx, details.UUID, result.macaroon)
	}

	return nil
}

// handleRelationDying notifies the offerer relation worker that the relation
// is dying, so it can clean up any resources associated with it.
func (w *localConsumerWorker) handleRelationDying(ctx context.Context, relationUUID corerelation.UUID, mac *macaroon.Macaroon) error {
	w.logger.Debugf(ctx, "relation %q is dying", relationUUID)

	change := params.RemoteRelationChangeEvent{
		RelationToken:           relationUUID.String(),
		Life:                    life.Dying,
		ApplicationOrOfferToken: w.applicationUUID.String(),
		Macaroons:               macaroon.Slice{mac},
		BakeryVersion:           bakery.LatestVersion,

		// NOTE (stickupkid): Work out if we need to pass ForceCleanup here.
		// I suspect that because the relation never transitions to dead, and
		// if we restart the worker, then we'll get a RelationNotFound error.
		// Thus ForceCleanup will never be true.
	}

	if err := w.remoteModelClient.PublishRelationChange(ctx, change); isNotFound(err) {
		w.logger.Debugf(ctx, "relation %q dying, but offerer side already removed", relationUUID)
		return nil
	} else if err != nil {
		if terminalErr := w.handleDischargeRequiredWhilstDying(ctx, err, relationUUID); terminalErr != nil {
			return terminalErr
		}
		return errors.Annotatef(err, "notifying offerer relation worker that relation %q is dying", relationUUID)
	}

	return nil
}

func (w *localConsumerWorker) handleDischargeRequiredWhilstDying(ctx context.Context, err error, relationUUID corerelation.UUID) error {
	if params.ErrCode(err) != params.CodeDischargeRequired {
		return nil
	}

	// If consume permission has been revoked for the offer, set the
	// status of the local remote application entity.
	if err := w.crossModelService.SetRemoteApplicationOffererStatus(ctx, w.applicationName, status.StatusInfo{
		Status:  status.Error,
		Message: fmt.Sprintf("offer permission revoked: %v", err.Error()),
	}); err != nil {
		w.logger.Errorf(ctx,
			"updating remote application %q status from remote model %q: %v",
			w.applicationName, w.offererModelUUID, err)
	}

	// Tear down the relation and unit workers, we're in a dying state, and the
	// offerer macaroon is no longer valid and failed to discharge. Remove all
	// the workers and start the process again, which will re-establish the
	// relation if it is still valid.
	//
	// We won't ever be able to tell the offerer side that the relation has gone
	// away, because if we bounce, we won't ever be in this state again. This
	// won't re-establish itself on the consumer side, because the relation
	// doesn't exist. The only way to recover this is to remove the offerer
	// hidden relation.
	return internalerrors.Errorf("%w: relation %q", ErrPermissionRevokedWhilstDying, relationUUID)
}

// Create a new worker to watch for changes to the relation in the offering
// model.
//
// The worker is identified by the relation UUID, so it can track that
// information, along with the offering application UUID and macaroon used to
// access the relation in the offering model. If we have this information, we
// should be able to pin point the relation in the offering model.
func (w *localConsumerWorker) ensureOffererRelationWorker(
	ctx context.Context,
	relationUUID corerelation.UUID,
	offererApplicationUUID application.UUID,
	mac *macaroon.Macaroon,
) error {
	name := fmt.Sprintf("offerer-relation:%s", relationUUID)
	if err := w.runner.StartWorker(ctx, name, func(ctx context.Context) (worker.Worker, error) {
		return w.newOffererRelationsWorker(offererrelations.Config{
			Client:                 w.remoteModelClient,
			ConsumerRelationUUID:   relationUUID,
			OffererApplicationUUID: offererApplicationUUID,
			Macaroon:               mac,
			Changes:                w.offererRelationChanges,
			Clock:                  w.clock,
			Logger:                 w.logger.Child("offerer-relation"),
		})

	}); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return errors.Annotatef(err, "starting offerer relation worker for %q", relationUUID)
	}

	return nil
}

// Ensure the unit relation workers for both the consumer and offerer sides
// of the relation.
// This is idempotent, if the workers already exist, they will not be created
// again.
func (w *localConsumerWorker) ensureUnitRelationWorkers(
	ctx context.Context,
	details relation.RelationDetails,
	offferApplicationUUID application.UUID,
	mac *macaroon.Macaroon,
) error {
	consumerName := fmt.Sprintf("consumer-unit-relation:%s", details.UUID)
	if err := w.runner.StartWorker(ctx, consumerName, func(ctx context.Context) (worker.Worker, error) {
		return w.newConsumerUnitRelationsWorker(consumerunitrelations.Config{
			Service:                 w.crossModelService,
			ConsumerApplicationUUID: w.applicationUUID,
			ConsumerRelationUUID:    details.UUID,
			Changes:                 w.consumerRelationUnitChanges,
			Clock:                   w.clock,
			Logger:                  w.logger.Child("consumer-unit"),
		})
	}); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return errors.Annotatef(err, "starting consumer unit relation worker for %q", details.UUID)
	}

	offererName := fmt.Sprintf("offerer-unit-relation:%s", details.UUID)
	if err := w.runner.StartWorker(ctx, offererName, func(ctx context.Context) (worker.Worker, error) {
		return w.newOffererUnitRelationsWorker(offererunitrelations.Config{
			Client:                 w.remoteModelClient,
			ConsumerRelationUUID:   details.UUID,
			OffererApplicationUUID: offferApplicationUUID,
			Macaroon:               mac,
			Changes:                w.offererRelationUnitChanges,
			Clock:                  w.clock,
			Logger:                 w.logger.Child("offerer-unit"),
		})
	}); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return errors.Annotatef(err, "starting offerer unit relation worker for %q", details.UUID)
	}

	return nil
}

// registerConsumerRelation registers the relation in the offering model,
// and returns the offering application UUID and macaroon for the relation.
// The relation has been created in the consumer model.
func (w *localConsumerWorker) registerConsumerRelation(
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
		return consumerRelationResult{}, errors.New("no result from registering consumer relation in offerer model")
	} else if len(offererRelations) > 1 {
		w.logger.Warningf(ctx, "expected one result from registering consumer relation in offerer model, got %d", len(offererRelations))
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
	offererAppUUID, err := application.ParseID(registerResult.Token)
	if err != nil {
		return consumerRelationResult{}, errors.Annotatef(err, "parsing offering application token %q", registerResult.Token)
	}

	// We have a new macaroon attenuated to the relation.
	// Save for the firewaller.
	if err := w.crossModelService.SaveMacaroonForRelation(ctx, relationUUID, registerResult.Macaroon); err != nil {
		return consumerRelationResult{}, errors.Annotatef(err, "saving macaroon for %q", relationUUID)
	}

	return consumerRelationResult{
		offererApplicationUUID: offererAppUUID,
		macaroon:               registerResult.Macaroon,
	}, nil
}

type consumerRelationResult struct {
	offererApplicationUUID application.UUID
	macaroon               *macaroon.Macaroon
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
