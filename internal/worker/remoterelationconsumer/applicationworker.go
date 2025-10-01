// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"context"
	"fmt"
	"sync"

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
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/worker/remoterelationconsumer/localunitrelations"
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

	// crossModelService is the domain services used to interact with the local
	// database for cross model relations.
	crossModelService CrossModelService

	// remoteModelClient interacts with the remote (offering) model.
	remoteModelClient          RemoteModelRelationsClient
	remoteRelationClientGetter RemoteRelationClientGetter

	mu sync.Mutex
	// These attributes are relevant to dealing with a specific
	// remote application proxy.
	offerUUID       string
	applicationName string
	applicationUUID application.UUID
	localModelUUID  model.UUID // uuid of the model hosting the local application
	remoteModelUUID string     // uuid of the model hosting the remote offer
	consumeVersion  int

	secretChangesWatcher watcher.SecretsRevisionWatcher
	secretChanges        watcher.SecretRevisionChannel

	localRelationUnitChanges  chan relation.RelationUnitChange
	remoteRelationUnitChanges chan remoteunitrelations.RelationUnitChange
	remoteRelationChanges     chan remoterelations.RelationChange

	// relations is stored here for the engine report.
	relations map[string]*relationInfo

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

	w := &remoteApplicationWorker{
		crossModelService: config.CrossModelService,

		localRelationUnitChanges:  make(chan relation.RelationUnitChange),
		remoteRelationUnitChanges: make(chan remoteunitrelations.RelationUnitChange),
		remoteRelationChanges:     make(chan remoterelations.RelationChange),

		offerUUID:       config.OfferUUID,
		applicationName: config.ApplicationName,
		applicationUUID: config.ApplicationUUID,
		localModelUUID:  config.LocalModelUUID,
		remoteModelUUID: config.RemoteModelUUID,
		consumeVersion:  config.ConsumeVersion,
		offerMacaroon:   config.Macaroon,

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
	w.mu.Lock()
	defer w.mu.Unlock()

	relationsInfo := make(map[string]interface{})
	for rel, info := range w.relations {
		report := map[string]interface{}{
			"relation-id":       info.relationId,
			"local-dead":        info.localDead,
			"suspended":         info.suspended,
			"application-token": info.localApplicationToken,
			"relation-token":    info.localRelationToken,
			"local-endpoint":    info.localEndpoint.Name,
			"remote-endpoint":   info.remoteEndpointName,
		}
		if info.remoteRelationWorker != nil {
			report["last-status-event"] = info.remoteRelationWorker.Report()
		}
		if info.localUnitWorker != nil {
			report["last-local-change"] = info.localUnitWorker.Report()
		}
		if info.remoteUnitWorker != nil {
			report["last-remote-change"] = info.remoteUnitWorker.Report()
		}
		relationsInfo[rel] = report
	}
	if len(relationsInfo) > 0 {
		result["relations"] = relationsInfo
	}
	result["remote-model-uuid"] = w.remoteModelUUID
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
	offerStatusChanges := offerStatusWatcher.Changes()

	w.mu.Lock()
	w.relations = make(map[string]*relationInfo)
	w.mu.Unlock()
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

			// TODO (stickupkid): Pass the changes to the get relation details.
			results, err := w.crossModelService.GetRelationDetails(ctx, "")
			if err != nil {
				return errors.Annotate(err, "querying relations")
			}
			_ = results

			for _, result := range []params.RemoteRelationResult{} {
				key := ""
				if err := w.relationChanged(ctx, key, result); err != nil {
					if isNotFound(err) {
						// Relation has been deleted, so ensure relevant workers are stopped.
						w.logger.Debugf(ctx, "relation %q changed but has been removed", key)
						err2 := w.localRelationChanged(ctx, key, nil)
						if err2 != nil {
							return errors.Annotatef(err2, "cleaning up removed local relation %q", key)
						}
						continue
					}
					return errors.Annotatef(err, "handling change for relation %q", key)
				}
			}

		case change := <-w.localRelationUnitChanges:
			w.logger.Debugf(ctx, "local relation units changed -> publishing: %v", change)
			// TODO(babbageclunk): add macaroons to event here instead
			// of in the relation units worker.
			if err := w.remoteModelClient.PublishRelationChange(ctx, params.RemoteRelationChangeEvent{}); err != nil {
				w.checkOfferPermissionDenied(ctx, err, change.ApplicationOrOfferToken, change.RelationToken)
				if isNotFound(err) || params.IsCodeCannotEnterScope(err) {
					w.logger.Debugf(ctx, "relation %v changed but remote side already removed", change.Tag.Id())
					continue
				}
				return errors.Annotatef(err, "publishing relation change %#v to remote model %v", &change, w.remoteModelUUID)
			}

			if err := w.localRelationChanged(ctx, change.Tag.Id(), ptr(change.UnitCount)); err != nil {
				return errors.Annotatef(err, "processing local relation change for %v", change.Tag.Id())
			}

		case change := <-w.remoteRelationUnitChanges:
			w.logger.Debugf(ctx, "remote relation units changed -> consuming: %v", change)

			if err := w.crossModelService.ConsumeRemoteRelationChange(ctx); err != nil {
				if isNotFound(err) || params.IsCodeCannotEnterScope(err) {
					w.logger.Debugf(ctx, "relation %v changed but local side already removed", change.Tag.Id())
					continue
				}
				return errors.Annotatef(err, "consuming relation change %#v from remote model %v", &change, w.remoteModelUUID)
			}

		case change := <-w.remoteRelationChanges:
			w.logger.Debugf(ctx, "remote relations changed -> consuming: %v", change)

			if err := w.crossModelService.ConsumeRemoteRelationChange(ctx); err != nil {
				if isNotFound(err) || params.IsCodeCannotEnterScope(err) {
					w.logger.Debugf(ctx, "relation %v changed but local side already removed", change.Tag.Id())
					continue
				}
				return errors.Annotatef(err, "consuming relation change %#v from remote model %v", &change, w.remoteModelUUID)
			}

		case changes := <-offerStatusChanges:
			w.logger.Debugf(ctx, "offer status changed: %#v", changes)
			for _, change := range changes {
				if err := w.crossModelService.SetRemoteApplicationOffererStatus(ctx, w.applicationUUID, change.Status); err != nil {
					return errors.Annotatef(err, "updating remote application %v status from remote model %v", w.applicationName, w.remoteModelUUID)
				}

				// If the offer is terminated the status watcher can be stopped immediately.
				if change.Status.Status == status.Terminated {
					if err := worker.Stop(offerStatusWatcher); err != nil {
						w.logger.Warningf(ctx, "error stopping status watcher for saas application %s: %v", w.applicationName, err)
					}
					offerStatusChanges = nil
					break
				}
			}

		case changes := <-w.secretChanges:
			err := w.crossModelService.ConsumeRemoteSecretChanges(ctx)
			if err != nil {
				if isNotFound(err) {
					w.logger.Debugf(ctx, "secrets %v changed but local side already removed", changes)
					continue
				}
				return errors.Annotatef(err, "consuming secrets change %#v from remote model %v", changes, w.remoteModelUUID)
			}
		}
	}
}

func (w *remoteApplicationWorker) setupRemoteModelClient(ctx context.Context) (RemoteModelRelationsClient, error) {
	remoteModelClient, err := w.remoteRelationClientGetter.GetRemoteRelationClient(ctx, w.remoteModelUUID)
	if err == nil {
		return remoteModelClient, nil
	}

	// Attempt to set the status on the remote offer to indicate that
	// we cannot connect to the remote model. If this fails, log the error,
	// as we don't want the worker to bounce on this error. Instead, we want
	// to bounce on the original error.
	if err := w.crossModelService.SetRemoteApplicationOffererStatus(ctx, w.applicationUUID, status.StatusInfo{
		Status:  status.Error,
		Message: fmt.Sprintf("cannot connect to external controller: %v", err.Error()),
	}); err != nil {
		w.logger.Errorf(ctx, "failed updating remote application %v status from remote model %v: %v", w.applicationName, w.remoteModelUUID, err)
	}
	return nil, errors.Annotate(err, "cannot connect to external controller")
}

func (w *remoteApplicationWorker) watchRemoteOfferStatus(ctx context.Context) (watcher.OfferStatusWatcher, error) {
	offerStatusWatcher, err := w.remoteModelClient.WatchOfferStatus(ctx, params.OfferArg{
		OfferUUID:     w.offerUUID,
		Macaroons:     macaroon.Slice{w.offerMacaroon},
		BakeryVersion: bakery.LatestVersion,
	})
	if err != nil {
		w.checkOfferPermissionDenied(ctx, err, "", "")
		if isNotFound(err) {
			return nil, w.remoteOfferRemoved(ctx)
		}
		return nil, errors.Annotate(err, "watching status for offer")
	}

	if err := w.catacomb.Add(offerStatusWatcher); err != nil {
		return nil, errors.Trace(err)
	}

	return offerStatusWatcher, nil
}

func (w *remoteApplicationWorker) checkOfferPermissionDenied(ctx context.Context, err error, appToken, localRelationToken string) {
	// If consume permission has been revoked for the offer, set the
	// status of the local remote application entity.
	if params.ErrCode(err) != params.CodeDischargeRequired {
		return
	}

	if err := w.crossModelService.SetRemoteApplicationOffererStatus(ctx, w.applicationUUID, status.StatusInfo{
		Status:  status.Error,
		Message: err.Error(),
	}); err != nil {
		w.logger.Errorf(ctx,
			"updating remote application %v status from remote model %v: %v",
			w.applicationName, w.remoteModelUUID, err)
	}

	// If we don't have the tokens, we can't do anything more.
	if localRelationToken == "" {
		return
	}

	w.logger.Debugf(ctx, "discharge required error: app token: %v rel token: %v", appToken, localRelationToken)

	suspended := true
	event := params.RemoteRelationChangeEvent{
		RelationToken:           localRelationToken,
		ApplicationOrOfferToken: appToken,
		Suspended:               &suspended,
		SuspendedReason:         "offer permission revoked",
	}
	_ = event
	if err := w.crossModelService.ConsumeRemoteRelationChange(ctx); err != nil {
		w.logger.Errorf(ctx, "updating relation status: %v", err)
	}
}

func (w *remoteApplicationWorker) remoteOfferRemoved(ctx context.Context) error {
	w.logger.Debugf(ctx, "remote offer for %s has been removed", w.applicationName)

	// TODO (stickupkid): What's the point of setting the status to terminated,
	// and not removing any additional workers? If the offer has been removed,
	// surely we want to some how trigger a cleanup of the remote application
	// and data associated with it?
	if err := w.crossModelService.SetRemoteApplicationOffererStatus(ctx, w.applicationUUID, status.StatusInfo{
		Status:  status.Terminated,
		Message: "offer has been removed",
	}); err != nil {
		return errors.Annotatef(err, "updating remote application %v status from remote model %v", w.applicationName, w.remoteModelUUID)
	}
	return nil
}

func (w *remoteApplicationWorker) processRelationDying(ctx context.Context, key string, r *relationInfo, forceCleanup bool) error {
	w.logger.Debugf(ctx, "relation %v dying (%v)", key, forceCleanup)

	change := params.RemoteRelationChangeEvent{
		RelationToken:           r.localRelationToken,
		Life:                    life.Dying,
		ApplicationOrOfferToken: r.localApplicationToken,
		Macaroons:               macaroon.Slice{r.macaroon},
		BakeryVersion:           bakery.LatestVersion,
	}
	// forceCleanup will be true if the worker has restarted and because the relation had
	// already been removed, we won't get any more unit departed events.
	if forceCleanup {
		change.ForceCleanup = &forceCleanup
	}
	if err := w.remoteModelClient.PublishRelationChange(ctx, change); err != nil {
		w.checkOfferPermissionDenied(ctx, err, r.localApplicationToken, r.localRelationToken)
		if isNotFound(err) {
			w.logger.Debugf(ctx, "relation %v dying but remote side already removed", key)
			return nil
		}
		return errors.Annotatef(err, "publishing relation dying %#v to remote model %v", &change, w.remoteModelUUID)
	}

	return nil
}

func (w *remoteApplicationWorker) processRelationSuspended(ctx context.Context, key string, relLife life.Value, relations map[string]*relationInfo) error {
	w.logger.Debugf(ctx, "(%v) relation %v suspended", relLife, key)
	relation, ok := relations[key]
	if !ok {
		return nil
	}

	// Only stop the watchers for relation unit changes if relation is alive,
	// as we need to always deal with units leaving scope etc if the relation is dying.
	if relLife != life.Alive {
		return nil
	}

	if relation.localUnitWorker != nil {
		if err := worker.Stop(relation.localUnitWorker); err != nil {
			w.logger.Warningf(ctx, "stopping local relation unit worker for %v: %v", key, err)
		}
		relation.localUnitWorker = nil
	}
	if relation.remoteUnitWorker != nil {
		if err := worker.Stop(relation.remoteUnitWorker); err != nil {
			w.logger.Warningf(ctx, "stopping remote relation unit worker for %v: %v", key, err)
		}
		relation.remoteUnitWorker = nil
	}
	return nil
}

// processLocalRelationRemoved is called when a change event arrives from the remote model
// but the relation in the local model has been removed.
func (w *remoteApplicationWorker) processLocalRelationRemoved(ctx context.Context, key string, relations map[string]*relationInfo) error {
	w.logger.Debugf(ctx, "local relation %v removed", key)
	relation, ok := relations[key]
	if !ok {
		return nil
	}

	// Stop the worker which watches remote status/life.
	if relation.remoteRelationWorker != nil {
		if err := worker.Stop(relation.remoteRelationWorker); err != nil {
			w.logger.Warningf(ctx, "stopping remote relations worker for %v: %v", key, err)
		}
		relation.remoteRelationWorker = nil
		relations[key] = relation
	}

	w.logger.Debugf(ctx, "remote relation %v removed from local model", key)
	return nil
}

// localRelationChanged processes changes to the relation
// as recorded in the local model; the primary function
// is to shut down workers when the relation is dead.
func (w *remoteApplicationWorker) localRelationChanged(ctx context.Context, key string, unitCountPtr *int) error {
	unitCountMsg := " (removed)"
	if unitCountPtr != nil {
		unitCountMsg = fmt.Sprintf(", still has %d unit(s) in scope", *unitCountPtr)
	}
	w.logger.Debugf(ctx, "local relation %v changed%s", key, unitCountMsg)
	w.mu.Lock()
	defer w.mu.Unlock()

	relation, ok := w.relations[key]
	if !ok {
		w.logger.Debugf(ctx, "local relation %v already gone", key)
		return nil
	}
	w.logger.Debugf(ctx, "relation %v in mem unit count is %d", key, relation.localUnitCount)
	if unitCountPtr != nil {
		relation.localUnitCount = *unitCountPtr
	}
	if !relation.localDead {
		w.logger.Debugf(ctx, "local relation %v not dead yet", key)
		return nil
	}
	if relation.localUnitCount > 0 {
		w.logger.Debugf(ctx, "relation dead but still has %d units in scope", relation.localUnitCount)
		return nil
	}
	return w.terminateLocalRelation(ctx, key)
}

func (w *remoteApplicationWorker) terminateLocalRelation(ctx context.Context, key string) error {
	relation, ok := w.relations[key]
	if !ok {
		return nil
	}
	delete(w.relations, key)
	w.logger.Debugf(ctx, "local relation %v is terminated", key)

	// For the unit watchers, check to see if these are nil before stopping.
	// They will be nil if the relation was suspended and then we kill it for real.
	if relation.localUnitWorker != nil {
		if err := worker.Stop(relation.localUnitWorker); err != nil {
			w.logger.Warningf(ctx, "stopping local relation unit worker for %v: %v", key, err)
		}
		relation.localUnitWorker = nil
	}
	if relation.remoteUnitWorker != nil {
		if err := worker.Stop(relation.remoteUnitWorker); err != nil {
			w.logger.Warningf(ctx, "stopping remote relation unit worker for %v: %v", key, err)
		}
		relation.remoteUnitWorker = nil
	}

	w.logger.Debugf(ctx, "local relation %v removed from local model", key)
	return nil
}

// relationChanged processes changes to the relation as recorded in the
// local model when a change event arrives from the remote model.
func (w *remoteApplicationWorker) relationChanged(ctx context.Context, key string, localRelation params.RemoteRelationResult) (err error) {
	w.logger.Debugf(ctx, "relation %q changed in local model: %#v", key, localRelation)
	w.mu.Lock()
	defer w.mu.Unlock()

	defer func() {
		if err == nil || !isNotFound(err) {
			return
		}
		if err2 := w.processLocalRelationRemoved(ctx, key, w.relations); err2 != nil {
			err = errors.Annotate(err2, "processing local relation removed")
		}
		if r := w.relations[key]; r != nil {
			r.localDead = true
			w.relations[key] = r
		}
	}()
	if localRelation.Error != nil {
		return localRelation.Error
	}
	localRelationInfo := localRelation.Result

	// If we have previously started the watcher and the
	// relation is now suspended, stop the watcher.
	if r := w.relations[key]; r != nil {
		wasSuspended := r.suspended
		r.suspended = localRelationInfo.Suspended
		w.relations[key] = r
		if localRelationInfo.Suspended {
			return w.processRelationSuspended(ctx, key, localRelationInfo.Life, w.relations)
		}
		if localRelationInfo.Life == life.Alive {
			if r.localDead {
				// A previous relation with the same name was removed but
				// not cleaned up properly so do it now before starting up
				// workers again.
				w.logger.Debugf(ctx, "still have zombie local relation %v", key)
				if err := w.terminateLocalRelation(ctx, key); err != nil {
					return errors.Annotatef(err, "terminating zombie local relation %v", key)
				}
			} else if !wasSuspended {
				// Nothing to do, we have previously started the watcher.
				return nil
			}
		}
	}

	return w.processConsumingRelation(ctx, key, localRelationInfo)
}

// startUnitsWorkers starts 2 workers to watch for unit settings or departed changes;
// one worker is for the local model, the other for the remote model.
func (w *remoteApplicationWorker) startUnitsWorkers(
	ctx context.Context,
	relationTag names.RelationTag,
	localRelationToken, remoteAppToken string,
	applicationName string,
	mac *macaroon.Macaroon,
) (ReportableWorker, ReportableWorker, error) {
	localUnitsWorker, err := w.newLocalUnitRelationsWorker(localunitrelations.Config{
		Service:     w.crossModelService,
		RelationTag: relationTag,
		Macaroon:    mac,
		Changes:     w.localRelationUnitChanges,
		Clock:       w.clock,
		Logger:      w.logger,
	})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if err := w.catacomb.Add(localUnitsWorker); err != nil {
		return nil, nil, errors.Trace(err)
	}

	remoteUnitsWorker, err := w.newRemoteUnitRelationsWorker(remoteunitrelations.Config{
		Client:         w.remoteModelClient,
		RelationTag:    relationTag,
		Macaroon:       mac,
		RemoteAppToken: remoteAppToken,
		Changes:        w.remoteRelationUnitChanges,
		Clock:          w.clock,
		Logger:         w.logger,
	})
	if err != nil {
		w.checkOfferPermissionDenied(ctx, err, remoteAppToken, localRelationToken)
		return nil, nil, errors.Trace(err)
	}
	if err := w.catacomb.Add(remoteUnitsWorker); err != nil {
		return nil, nil, errors.Trace(err)
	}
	return localUnitsWorker, remoteUnitsWorker, nil
}

// processConsumingRelation starts the sub-workers necessary to listen and publish
// local unit settings changes, and watch and consume remote unit settings changes.
// Ths will be called when a new relation is created or when a relation resumes
// after being suspended.
func (w *remoteApplicationWorker) processConsumingRelation(
	ctx context.Context,
	key string,
	remoteRelation *params.RemoteRelation,
) error {

	// We have not seen the relation before, make
	// sure it is registered on the offering side.
	// Or relation was suspended and is now resumed so re-register.
	applicationTag := names.NewApplicationTag(remoteRelation.ApplicationName)
	relationTag := names.NewRelationTag(key)
	localApplicationToken, remoteAppToken, localRelationToken, mac, err := w.registerRemoteRelation(
		ctx,
		applicationTag, relationTag, w.offerUUID, w.consumeVersion,
		remoteRelation.Endpoint, remoteRelation.RemoteEndpointName)
	if err != nil {
		w.checkOfferPermissionDenied(ctx, err, "", "")
		return errors.Annotatef(err, "registering application %v and relation %v", remoteRelation.ApplicationName, relationTag.Id())
	}
	w.logger.Debugf(ctx, "remote relation registered for %q: app token=%q, rel token=%q, remote app token=%q", key, localApplicationToken, localRelationToken, remoteAppToken)

	// Have we seen the relation before.
	r, relationKnown := w.relations[key]
	if !relationKnown {
		remoteRelationsWorker, err := w.newRemoteRelationsWorker(remoterelations.Config{
			RelationTag:         relationTag,
			ApplicationToken:    remoteAppToken,
			LocalRelationToken:  localRelationToken,
			RemoteRelationToken: remoteAppToken,
			Changes:             w.remoteRelationChanges,
			Clock:               w.clock,
			Logger:              w.logger,
		})
		if err != nil {
			return errors.Trace(err)
		}
		if err := w.catacomb.Add(remoteRelationsWorker); err != nil {
			return errors.Trace(err)
		}
		r = &relationInfo{
			relationId:            remoteRelation.Id,
			suspended:             remoteRelation.Suspended,
			localUnitCount:        remoteRelation.UnitCount,
			remoteRelationWorker:  remoteRelationsWorker,
			macaroon:              mac,
			localEndpoint:         remoteRelation.Endpoint,
			remoteEndpointName:    remoteRelation.RemoteEndpointName,
			localApplicationToken: localApplicationToken,
			localRelationToken:    localRelationToken,
		}
		w.relations[key] = r
	}

	if r.localUnitWorker == nil && !remoteRelation.Suspended {
		// Also start the units watchers (local and remote).
		localUnitsWorker, remoteUnitsWorker, err := w.startUnitsWorkers(
			ctx,
			relationTag, localRelationToken, remoteAppToken, remoteRelation.ApplicationName,
			mac)
		if err != nil {
			return errors.Annotate(err, "starting relation units workers")
		}
		r.localUnitWorker = localUnitsWorker
		r.remoteUnitWorker = remoteUnitsWorker
	}

	if w.secretChangesWatcher == nil {
		w.secretChangesWatcher, err = w.remoteModelClient.WatchConsumedSecretsChanges(ctx, localApplicationToken, localRelationToken, w.offerMacaroon)
		if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, errors.NotImplemented) {
			w.checkOfferPermissionDenied(ctx, err, "", "")
			return errors.Annotate(err, "watching consumed secret changes")
		}
		if err == nil {
			if err := w.catacomb.Add(w.secretChangesWatcher); err != nil {
				return errors.Trace(err)
			}
			w.secretChanges = w.secretChangesWatcher.Changes()
		}
	}

	// If the relation is dying, stop the watcher.
	if remoteRelation.Life != life.Alive {
		return w.processRelationDying(ctx, key, r, !relationKnown)
	}

	return nil
}

func (w *remoteApplicationWorker) registerRemoteRelation(
	ctx context.Context,
	applicationTag, relationTag names.Tag, offerUUID string, consumeVersion int,
	localEndpointInfo params.RemoteEndpoint, remoteEndpointName string,
) (localApplicationToken, offeringAppToken, localRelationToken string, _ *macaroon.Macaroon, _ error) {
	w.logger.Debugf(ctx, "register remote relation %v to local application %v", relationTag.Id(), applicationTag.Id())

	fail := func(err error) (string, string, string, *macaroon.Macaroon, error) {
		return "", "", "", nil, err
	}

	// Ensure the relation is exported first up.
	localApplicationToken, localRelationToken, err := w.crossModelService.ExportApplicationAndRelationToken(ctx, applicationTag, relationTag)
	if err != nil && !errors.Is(err, errors.AlreadyExists) {
		return fail(errors.Annotatef(err, "exporting relation %v and application %v", relationTag, applicationTag))
	}

	// This data goes to the remote model so we map local info
	// from this model to the remote arg values and visa versa.
	arg := params.RegisterRemoteRelationArg{
		ApplicationToken:  localApplicationToken,
		SourceModelTag:    names.NewModelTag(w.localModelUUID.String()).String(),
		RelationToken:     localRelationToken,
		OfferUUID:         offerUUID,
		RemoteEndpoint:    localEndpointInfo,
		LocalEndpointName: remoteEndpointName,
		ConsumeVersion:    consumeVersion,
	}
	if w.offerMacaroon != nil {
		arg.Macaroons = macaroon.Slice{w.offerMacaroon}
		arg.BakeryVersion = bakery.LatestVersion
	}
	remoteRelation, err := w.remoteModelClient.RegisterRemoteRelations(ctx, arg)
	if err != nil {
		return fail(errors.Trace(err))
	}
	// remoteAppIds is a slice but there's only one item
	// as we currently only register one remote application
	if err := remoteRelation[0].Error; err != nil {
		return fail(errors.Annotatef(err, "registering relation %v", relationTag))
	}
	// Import the application UUID from the offering model.
	registerResult := *remoteRelation[0].Result
	offeringAppToken = registerResult.Token

	// We have a new macaroon attenuated to the relation.
	// Save for the firewaller.
	if err := w.crossModelService.SaveMacaroonForRelation(ctx, relationTag, registerResult.Macaroon); err != nil {
		return fail(errors.Annotatef(
			err, "saving macaroon for %v", relationTag))
	}

	appTag := names.NewApplicationTag(w.applicationName)
	w.logger.Debugf(ctx, "import remote application token %v for %v", offeringAppToken, w.applicationName)
	err = w.crossModelService.ImportRemoteApplicationToken(ctx, appTag, offeringAppToken)
	if err != nil && !errors.Is(err, errors.AlreadyExists) {
		return fail(errors.Annotatef(
			err, "importing remote application %v to local model", w.applicationName))
	}
	return localApplicationToken, offeringAppToken, localRelationToken, registerResult.Macaroon, nil
}

// relationInfo holds attributes relevant to a particular relation between a
// local app and a remote offer.
type relationInfo struct {
	relationId     int
	localDead      bool
	suspended      bool
	localUnitCount int

	localUnitWorker      ReportableWorker
	remoteUnitWorker     ReportableWorker
	remoteRelationWorker ReportableWorker

	localApplicationToken string // token for app in local model
	localRelationToken    string // token for relation in local model
	localEndpoint         params.RemoteEndpoint
	remoteEndpointName    string
	macaroon              *macaroon.Macaroon
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

func ptr[T any](v T) *T {
	return &v
}
