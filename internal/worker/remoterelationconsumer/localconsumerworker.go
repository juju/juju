// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	"fmt"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
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
	"github.com/juju/juju/core/unit"
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

const (
	// defaultBakeryVersion is the default bakery version to use when
	// communicating with the offerer model.
	defaultBakeryVersion = bakery.LatestVersion
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

	consumerRelationUnitChanges chan consumerunitrelations.RelationUnitChange
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
		Name: "local-consumer",
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

		consumerRelationUnitChanges: make(chan consumerunitrelations.RelationUnitChange),
		offererRelationUnitChanges:  make(chan offererunitrelations.RelationUnitChange),
		offererRelationChanges:      make(chan offererrelations.RelationChange),

		newConsumerUnitRelationsWorker: config.NewConsumerUnitRelationsWorker,
		newOffererUnitRelationsWorker:  config.NewOffererUnitRelationsWorker,
		newOffererRelationsWorker:      config.NewOffererRelationsWorker,

		clock:  config.Clock,
		logger: config.Logger,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "local-consumer",
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
	relationsWatcher, err := w.crossModelService.WatchRelationsLifeSuspendedStatusForApplication(ctx, w.applicationUUID)
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

	// Store a reference to the channel here as it might become nil if the offer
	// is terminated.
	offerStatusWatcherChanges := offerStatusWatcher.Changes()

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
					if err := w.handleRelationRemoved(ctx, relationUUID, 0); err != nil {
						// If we fail to remove the relation, we must kill the
						// worker, as nothing will come around and try again.
						// Thus, kill it and force the application worker to
						// restart and start afresh.
						return errors.Annotatef(err, "cleaning up removed relation %q", relationUUID)
					}

					// Nothing to see here, wait for the next change.
					continue

				} else if err != nil {
					return errors.Annotate(err, "querying relations")
				}

				// The relation changed, we need to process the changed details.
				if err := w.handleConsumerRelationChange(ctx, details); err != nil {
					return errors.Annotatef(err, "handling change for relation %q", relationUUID)
				}
			}

		case change := <-w.consumerRelationUnitChanges:
			w.logger.Debugf(ctx, "consumer relation units changed: %v", change)

			if err := w.handleConsumerUnitChange(ctx, change); err != nil {
				return errors.Annotatef(err, "handling consumer unit relation change for %q", change.RelationUUID)
			}

		case change := <-w.offererRelationUnitChanges:
			w.logger.Debugf(ctx, "offerer relation units changed: %v", change)

			if err := w.handleOffererRelationUnitChange(ctx, change); err != nil {
				return errors.Annotatef(err, "handling offerer unit relation change for %q", change.ConsumerRelationUUID)
			}

		case change := <-w.offererRelationChanges:
			w.logger.Debugf(ctx, "offerer relation changed: %v", change)

			if err := w.handleOffererRelationChange(ctx, change); err != nil {
				return errors.Annotatef(err, "handling offerer relation change for %q", change.ConsumerRelationUUID)
			}

		case changes, ok := <-offerStatusWatcherChanges:
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
				// If the offer has been terminated remove the remote
				// application offerer from the consumer model and stop the
				// worker.
				if change.Status.Status == status.Terminated {
					if _, err := w.crossModelService.RemoveRemoteApplicationOffererByApplicationUUID(ctx, w.applicationUUID, true, time.Minute); err != nil {
						return errors.Annotatef(err, "removing remote application offerer for %q", w.applicationName)
					}

					return RemoteApplicationOffererDeadErr
				}

				if err := w.crossModelService.SetRemoteApplicationOffererStatus(ctx, w.applicationName, change.Status); err != nil {
					return errors.Annotatef(err, "updating remote application %v status from remote model %v", w.applicationName, w.offererModelUUID)
				}
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
		BakeryVersion: defaultBakeryVersion,
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

func (w *localConsumerWorker) handleRelationRemoved(ctx context.Context, relationUUID corerelation.UUID, inScopeUnits int) error {
	w.logger.Debugf(ctx, "relation %q removed", relationUUID)

	// Remove the unit workers tracking the relation. The relation is now
	// transitioning, and we don't want the workers to be triggering changes.
	if dead, err := w.isRelationWorkerDead(ctx, relationUUID); err != nil {
		return errors.Annotatef(err, "querying offerer relation worker for %q", relationUUID)
	} else if !dead {
		w.logger.Debugf(ctx, "consumer relation %q is not dead", relationUUID)
		return nil
	}

	// If we have in-scope units, then the relation is still active, and we
	// don't need to do anything else.
	if inScopeUnits > 0 {
		w.logger.Debugf(ctx, "consumer relation %q is dead, but has in-scope units: %v", relationUUID, inScopeUnits)
		return nil
	}

	// The relation is dead, and has no in-scope units. We can remove the
	// unit relation workers.
	if err := w.removeConsumerUnitRelationWorkers(ctx, relationUUID); err != nil {
		w.logger.Warningf(ctx, "removing consumer unit relation workers for %q: %v", relationUUID, err)
	}

	return nil
}

// handleConsumerRelationChange processes changes to the relation as recorded in
// the local model when a change event arrives from the remote model.
func (w *localConsumerWorker) handleConsumerRelationChange(ctx context.Context, details relation.RelationDetails) error {
	w.logger.Debugf(ctx, "relation %q changed in local model: %v", details.UUID, details)

	switch {
	case details.Suspended:
		return w.handleConsumerRelationSuspended(ctx, details)
	default:
		return w.handleRelationConsumption(ctx, details)
	}
}

func (w *localConsumerWorker) handleConsumerRelationSuspended(ctx context.Context, details relation.RelationDetails) error {
	w.logger.Debugf(ctx, "relation %q is suspended, there are %d units in-scope", details.UUID, details.InScopeUnits)

	// If the relation is dying.
	if dead, err := w.isRelationWorkerDead(ctx, details.UUID); err != nil {
		return errors.Annotatef(err, "querying offerer relation worker for %q", details.UUID)
	} else if dead {
		return nil
	}

	// Only stop the watchers for relation unit changes if relation is alive, as
	// we need to always deal with units leaving scope etc if the relation is
	// dying.
	if details.Life != life.Alive {
		return nil
	}

	// Remove the unit watchers for both sides of the relation.
	if err := w.removeConsumerUnitRelationWorkers(ctx, details.UUID); err != nil {
		w.logger.Warningf(ctx, "removing consumer unit relation workers for %q: %v", details.UUID, err)
	}

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
		details.UUID, w.offerUUID,
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
	relationKnown, err := w.ensureOffererRelationWorker(ctx, details.UUID, result.offererApplicationUUID, result.macaroon)
	if err != nil {
		return errors.Annotatef(err, "creating offerer relation worker for %q", details.UUID)
	}

	// Create the unit watchers for both the consumer and offerer sides if the
	// relation is not suspended. It is expected that the unit watchers will
	// clean themselves up if the relation is suspended or removed.
	if err := w.ensureUnitRelationWorkers(ctx, details, result.offererApplicationUUID, result.macaroon); err != nil {
		return errors.Annotatef(err, "creating unit relation workers for %q", details.UUID)
	}

	// Handle the case where the relation is dying, and ensure we have no
	// workers still running for it.
	if details.Life != life.Alive {
		return w.handleRelationDying(ctx, details.UUID, result.macaroon, !relationKnown)
	}

	return nil
}

// handleRelationDying "notifies the offering model that the relation is dying,
// so it can clean up any resources associated with it.
func (w *localConsumerWorker) handleRelationDying(
	ctx context.Context,
	relationUUID corerelation.UUID,
	mac *macaroon.Macaroon,
	forceCleanup bool,
) error {
	w.logger.Debugf(ctx, "relation %q is dying", relationUUID)

	change := params.RemoteRelationChangeEvent{
		RelationToken:           relationUUID.String(),
		Life:                    life.Dying,
		ApplicationOrOfferToken: w.applicationUUID.String(),
		Macaroons:               macaroon.Slice{mac},
		BakeryVersion:           defaultBakeryVersion,
	}

	// forceCleanup will be true if the worker has restarted and because the
	// relation had already been removed, we won't get any more unit departed
	// events.
	if forceCleanup {
		change.ForceCleanup = ptr(true)
	}
	if err := w.remoteModelClient.PublishRelationChange(ctx, change); isNotFound(err) {
		w.logger.Debugf(ctx, "relation %q dying, but offerer side already removed", relationUUID)
		return nil
	} else if err != nil {
		if terminalErr := w.handleDischargeRequiredErrorWhilstDying(ctx, err, relationUUID); terminalErr != nil {
			return terminalErr
		}
		return errors.Annotatef(err, "notifying offerer relation worker that relation %q is dying", relationUUID)
	}

	return nil
}

// Create a new worker to watch for changes to the relation in the offering
// model.
//
// The worker is identified by the relation UUID, so it can track that
// information, along with the offering application UUID and macaroon used to
// access the relation in the offering model. If we have this information, we
// should be able to pin point the relation in the offering model.
//
// The boolean indicates if the worker was newly created (true), or already
// existed (false).
func (w *localConsumerWorker) ensureOffererRelationWorker(
	ctx context.Context,
	relationUUID corerelation.UUID,
	offererApplicationUUID application.UUID,
	mac *macaroon.Macaroon,
) (known bool, _ error) {
	if err := w.runner.StartWorker(ctx, offererRelationWorkerName(relationUUID), func(ctx context.Context) (worker.Worker, error) {
		return w.newOffererRelationsWorker(offererrelations.Config{
			Client:                 w.remoteModelClient,
			ConsumerRelationUUID:   relationUUID,
			OffererApplicationUUID: offererApplicationUUID,
			Macaroon:               mac,
			Changes:                w.offererRelationChanges,
			Clock:                  w.clock,
			Logger:                 w.logger.Child("offerer-relation"),
		})
	}); errors.Is(err, errors.AlreadyExists) {
		return true, nil
	} else if err != nil {
		return false, errors.Annotatef(err, "starting offerer relation worker for %q", relationUUID)
	}

	return false, nil
}

// Ensure the unit relation workers for both the consumer and offerer sides
// of the relation.
// This is idempotent, if the workers already exist, they will not be created
// again.
func (w *localConsumerWorker) ensureUnitRelationWorkers(
	ctx context.Context,
	details relation.RelationDetails,
	offerApplicationUUID application.UUID,
	mac *macaroon.Macaroon,
) error {
	if err := w.runner.StartWorker(ctx, consumerUnitRelationWorkerName(details.UUID), func(ctx context.Context) (worker.Worker, error) {
		return w.newConsumerUnitRelationsWorker(consumerunitrelations.Config{
			Service:                 w.crossModelService,
			ConsumerApplicationUUID: w.applicationUUID,
			ConsumerRelationUUID:    details.UUID,
			Macaroon:                mac,
			Changes:                 w.consumerRelationUnitChanges,
			Clock:                   w.clock,
			Logger:                  w.logger.Child("consumer-unit"),
		})
	}); err != nil && !errors.Is(err, errors.AlreadyExists) {
		return errors.Annotatef(err, "starting consumer unit relation worker for %q", details.UUID)
	}

	if err := w.runner.StartWorker(ctx, offererUnitRelationWorkerName(details.UUID), func(ctx context.Context) (worker.Worker, error) {
		return w.newOffererUnitRelationsWorker(offererunitrelations.Config{
			Client:                 w.remoteModelClient,
			ConsumerRelationUUID:   details.UUID,
			OffererApplicationUUID: offerApplicationUUID,
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
	relationUUID corerelation.UUID, offerUUID string,
	applicationEndpointIdent relation.Endpoint, remoteEndpointName string,
) (consumerRelationResult, error) {
	w.logger.Debugf(ctx, "register consumer relation %q to synthetic offerer application %q", relationUUID, w.applicationName)

	// This data goes to the remote model so we map local info
	// from this model to the remote arg values and visa versa.
	arg := params.RegisterConsumingRelationArg{
		ConsumerApplicationToken: w.applicationUUID.String(),
		SourceModelTag:           names.NewModelTag(w.consumerModelUUID.String()).String(),
		RelationToken:            relationUUID.String(),
		OfferUUID:                offerUUID,
		ConsumerApplicationEndpoint: params.RemoteEndpoint{
			Name:      applicationEndpointIdent.String(),
			Role:      applicationEndpointIdent.Role,
			Interface: applicationEndpointIdent.Interface,
			Limit:     applicationEndpointIdent.Limit,
		},
		OfferEndpointName: remoteEndpointName,
		ConsumeVersion:    w.consumeVersion,
		Macaroons:         macaroon.Slice{w.offerMacaroon},
		BakeryVersion:     defaultBakeryVersion,
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
	offererAppUUID, err := application.ParseUUID(registerResult.Token)
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

func (w *localConsumerWorker) handleConsumerUnitChange(ctx context.Context, change consumerunitrelations.RelationUnitChange) error {
	// Workout which units are no longer in scope and should be considered
	// as departed.
	departedUnits := set.NewInts(change.AllUnits...).Difference(set.NewInts(change.InScopeUnits...)).SortedValues()

	// Create the event to send to the offering model.
	event := params.RemoteRelationChangeEvent{
		RelationToken:           change.RelationUUID.String(),
		ApplicationOrOfferToken: w.applicationUUID.String(),
		ApplicationSettings:     convertSettingsMap(change.ApplicationSettings),

		ChangedUnits: transform.Slice(change.UnitsSettings, func(v relation.UnitSettings) params.RemoteRelationUnitChange {
			return params.RemoteRelationUnitChange{
				UnitId:   v.UnitID,
				Settings: convertSettingsMap(v.Settings),
			}
		}),
		InScopeUnits: change.InScopeUnits,

		// The following are for backwards compatibility with 3.x controllers.
		DepartedUnits: departedUnits,
		UnitCount:     len(change.InScopeUnits),

		Macaroons:     macaroon.Slice{change.Macaroon},
		BakeryVersion: defaultBakeryVersion,
	}

	// Dispatch the change to the offering model. This will ensure that any
	// changes to the units in the consumer model are reflected in the offering
	// model.
	if err := w.remoteModelClient.PublishRelationChange(ctx, event); err != nil {
		if dischargeErr := w.handleDischargeRequiredError(ctx, err, change.RelationUUID); dischargeErr != nil {
			w.logger.Errorf(ctx, "discharge error handling for consumer unit change for relation %q: %v", change.RelationUUID, dischargeErr)
		}

		// If the relation no longer exists in the offering model, we should
		// expect to see a new event from the offerer relation worker, which will
		// clean up the local relation.
		if isNotFound(err) || params.IsCodeCannotEnterScope(err) {
			w.logger.Debugf(ctx, "relation %q no longer exists in offerer model", change.RelationUUID)
			return nil
		}
		return errors.Annotatef(err, "publishing consumer relation %q change to offerer", change.RelationUUID)
	}

	if err := w.handleRelationRemoved(ctx, change.RelationUUID, len(change.InScopeUnits)); err != nil {
		return errors.Annotatef(err, "handling potential removal of relation %q after consumer unit change", change.RelationUUID)
	}

	return nil
}

func (w *localConsumerWorker) removeConsumerUnitRelationWorkers(ctx context.Context, relationUUID corerelation.UUID) error {
	w.logger.Debugf(ctx, "consumer relation %q is dead and has no in-scope units, removing consumer unit workers", relationUUID)

	if err := w.runner.StopAndRemoveWorker(consumerUnitRelationWorkerName(relationUUID), ctx.Done()); err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Annotatef(err, "stopping consumer unit relation worker for %q", relationUUID)
	}
	if err := w.runner.StopAndRemoveWorker(offererUnitRelationWorkerName(relationUUID), ctx.Done()); err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Annotatef(err, "stopping offerer unit relation worker for %q", relationUUID)
	}

	w.logger.Debugf(ctx, "removed consumer unit relation workers for %q", relationUUID)

	return nil
}

func (w *localConsumerWorker) handleDischargeRequiredErrorWhilstDying(ctx context.Context, err error, relationUUID corerelation.UUID) error {
	if params.ErrCode(err) != params.CodeDischargeRequired {
		return nil
	}

	// If consume permission has been revoked for the offer, set the status of
	// of the local application entity representing the remote offerer.
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
	// doesn't exist. The only way to recover this is to remove the synthetic
	// relation from the offering mode.
	return internalerrors.Errorf("%w: relation %q", ErrPermissionRevokedWhilstDying, relationUUID)
}

func (w *localConsumerWorker) handleDischargeRequiredError(ctx context.Context, err error, relationUUID corerelation.UUID) error {
	if params.ErrCode(err) != params.CodeDischargeRequired {
		return nil
	}

	// Failed to discharge the macaroon, which means that the offerer side isn't
	// aware of the relation changes anymore. In that case, we need to put the
	// relation into a suspended state. This will hopefully prevent any further
	// changes to the relation without mirroring them to the offerer side.
	if err := w.crossModelService.SetRemoteRelationSuspendedState(ctx, relationUUID, true, "Offer permission revoked"); err != nil {
		return errors.Annotatef(err, "setting relation %q to suspended after discharge error", relationUUID)
	}

	return nil
}

func (w *localConsumerWorker) isRelationWorkerDead(ctx context.Context, relationUUID corerelation.UUID) (bool, error) {
	_, err := w.runner.Worker(offererRelationWorkerName(relationUUID), ctx.Done())
	if err != nil && !errors.Is(err, errors.NotFound) {
		return false, errors.Annotatef(err, "querying offerer relation worker for %q", relationUUID)
	} else if errors.Is(err, errors.NotFound) {
		return true, nil
	}
	return false, nil
}

func (w *localConsumerWorker) handleOffererRelationUnitChange(ctx context.Context, change offererunitrelations.RelationUnitChange) error {
	details, err := w.crossModelService.GetRelationDetails(ctx, change.ConsumerRelationUUID)
	if errors.Is(err, relationerrors.RelationNotFound) {
		return w.handleRelationRemoved(ctx, change.ConsumerRelationUUID, 0)
	} else if err != nil {
		return errors.Annotatef(err, "querying relation details for %q", change.ConsumerRelationUUID)
	}

	switch {
	case change.Life != life.Alive:
		return w.handleOffererRelationRemoved(ctx, change.ConsumerRelationUUID)

	case change.Suspended != details.Suspended:
		return w.handleOffererRelationSuspendedState(ctx, change.Suspended, change.SuspendedReason, details)
	}

	unitSettings, err := w.handleUnitSettings(ctx, change.ChangedUnits)
	if err != nil {
		return errors.Annotatef(err, "handling unit settings for relation %q", change.ConsumerRelationUUID)
	}

	// Process the relation application and unit settings changes.
	if err := w.crossModelService.SetRelationRemoteApplicationAndUnitSettings(
		ctx,
		w.applicationUUID,
		change.ConsumerRelationUUID,
		change.ApplicationSettings,
		unitSettings,
	); err != nil {
		return errors.Annotatef(err, "setting application and unit settings %q", change.ConsumerRelationUUID)
	}

	// We've got departed units, these need to leave scope.
	if err := w.handleDepartedUnits(ctx, change.ConsumerRelationUUID, change.DeprecatedDepartedUnits); err != nil {
		return errors.Annotatef(err, "handling departed units for relation %q", change.ConsumerRelationUUID)
	}

	return nil
}

func (w *localConsumerWorker) handleOffererRelationRemoved(ctx context.Context, relationUUID corerelation.UUID) error {
	w.logger.Debugf(ctx, "offerer relation %q is dying or dead, removing local consumer relation", relationUUID)

	// Remove the remote relation from the local model. This will ensure that
	// all the associated data is cleaned up for the relation. The synthetic
	// unit in the relation will also be removed as part of this process.
	_, err := w.crossModelService.RemoveRemoteRelation(ctx, relationUUID, false, 0)
	if err != nil && !errors.Is(err, relationerrors.RelationNotFound) {
		return errors.Annotatef(err, "removing remote relation %q", relationUUID)
	}

	// There is nothing else to do here, as the relation is gone. We can safely
	// ignore the settings or departed units associated with the relation.
	return nil
}

func (w *localConsumerWorker) handleOffererRelationSuspendedState(ctx context.Context, suspended bool, reason string, details relation.RelationDetails) error {
	w.logger.Debugf(ctx, "offerer relation %q is suspended, suspending local consumer relation", details.UUID)

	return w.crossModelService.SetRemoteRelationSuspendedState(ctx, details.UUID, suspended, reason)
}

func (w *localConsumerWorker) handleUnitSettings(
	ctx context.Context,
	unitChanges []offererunitrelations.UnitChange,
) (map[unit.Name]map[string]string, error) {
	// Ensure all units exist in the local model.
	units, err := transform.SliceOrErr(unitChanges, func(u offererunitrelations.UnitChange) (unit.Name, error) {
		return unit.NewNameFromParts(w.applicationName, u.UnitID)
	})
	if err != nil {
		return nil, errors.Annotatef(err, "parsing unit names")
	}

	// Ensure all the units exist in the local model, we'll need these upfront
	// before we can process the application and unit settings.
	if err := w.crossModelService.EnsureUnitsExist(ctx, w.applicationUUID, units); err != nil {
		return nil, errors.Annotatef(err, "ensuring units exist")
	}

	// Map the unit settings into a map keyed by unit name.
	unitSettings := make(map[unit.Name]map[string]string, len(unitChanges))
	for i, u := range unitChanges {
		unitName := units[i]

		if u.Settings == nil {
			unitSettings[unitName] = nil
			continue
		}

		unitSettings[unitName] = u.Settings
	}

	return unitSettings, nil
}

func (w *localConsumerWorker) handleDepartedUnits(ctx context.Context, relationUUID corerelation.UUID, departedUnits []int) error {
	for _, u := range departedUnits {
		unitName, err := unit.NewNameFromParts(w.applicationName, u)
		if err != nil {
			return errors.Annotatef(err, "parsing departed unit name %q", u)
		}

		// If the relation unit doesn't exist, then it has already been removed,
		// so we can skip it.
		relationUnitUUID, err := w.crossModelService.GetRelationUnitUUID(ctx, relationUUID, unitName)
		if errors.Is(err, relationerrors.RelationUnitNotFound) {
			continue
		} else if err != nil {
			return errors.Annotatef(err, "querying relation unit UUID for departed unit %q", unitName)
		}

		if err := w.crossModelService.LeaveScope(ctx, relationUnitUUID); err != nil {
			return errors.Annotatef(err, "removing departed unit %q", unitName)
		}
	}
	return nil
}

func (w *localConsumerWorker) handleOffererRelationChange(ctx context.Context, change offererrelations.RelationChange) error {
	// Handle the dying/dead case of the relation. We do this **after** setting
	// the settings, so that the removal of the relation doesn't prevent us from
	// setting the settings.
	if change.Life != life.Alive {
		// If the relation is dying or dead, then we're done here. The units
		// will have already transitioned to departed.
		_, err := w.crossModelService.RemoveRemoteRelation(ctx, change.ConsumerRelationUUID, false, 0)
		if errors.Is(err, relationerrors.RelationNotFound) {
			return nil
		}
		return err
	}

	// Handle the case where the relation has become (un)suspended.
	if err := w.crossModelService.SetRemoteRelationSuspendedState(ctx, change.ConsumerRelationUUID, change.Suspended, change.SuspendedReason); err != nil {
		return errors.Annotatef(err, "setting suspended state for relation %q", change.ConsumerRelationUUID)
	}

	return nil
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

func consumerUnitRelationWorkerName(relationUUID corerelation.UUID) string {
	return fmt.Sprintf("consumer-unit-relation:%s", relationUUID)
}

func offererUnitRelationWorkerName(relationUUID corerelation.UUID) string {
	return fmt.Sprintf("offerer-unit-relation:%s", relationUUID)
}

func offererRelationWorkerName(relationUUID corerelation.UUID) string {
	return fmt.Sprintf("offerer-relation:%s", relationUUID)
}

func convertSettingsMap(in map[string]string) map[string]any {
	// It's important that we return nil if the input is nil, as this indicates
	// that there are no settings. An empty map indicates that there are
	// settings, but they are all empty.
	if in == nil {
		return nil
	}

	// Transform map method always returns a non-nil map in the presence of
	// a non-nil input map.
	return transform.Map(in, func(k, v string) (string, any) {
		return k, v
	})
}

func ptr[T any](v T) *T {
	return &v
}
