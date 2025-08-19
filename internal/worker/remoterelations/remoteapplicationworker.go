// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// ReportableWorker is an interface that allows a worker to be reported
// on by the engine.
type ReportableWorker interface {
	worker.Worker
	Report() map[string]any
}

// remoteApplicationWorker listens for localChanges to relations
// involving a remote application, and publishes change to
// local relation units to the remote model. It also watches for
// changes originating from the offering model and consumes those
// in the local model.
type remoteApplicationWorker struct {
	catacomb catacomb.Catacomb

	mu sync.Mutex

	// These attributes are relevant to dealing with a specific
	// remote application proxy.
	offerUUID                 string
	applicationName           string // name of the remote application proxy in the local model
	localModelUUID            string // uuid of the model hosting the local application
	remoteModelUUID           string // uuid of the model hosting the remote offer
	isConsumerProxy           bool
	consumeVersion            int
	localRelationUnitChanges  chan RelationUnitChangeEvent
	remoteRelationUnitChanges chan RelationUnitChangeEvent
	secretChangesWatcher      watcher.SecretsRevisionWatcher
	secretChanges             watcher.SecretRevisionChannel

	// relations is stored here for the engine report.
	relations map[string]*relation

	// offerMacaroon is used to confirm that permission has been granted to consume
	// the remote application to which this worker pertains.
	offerMacaroon *macaroon.Macaroon

	// localModelFacade interacts with the local (consuming) model.
	localModelFacade RemoteRelationsFacade
	// remoteModelFacade interacts with the remote (offering) model.
	remoteModelFacade RemoteModelRelationsFacadeCloser

	newRemoteModelRelationsFacadeFunc newRemoteRelationsFacadeFunc

	logger logger.Logger
}

// relation holds attributes relevant to a particular
// relation between a local app and a remote offer.
type relation struct {
	relationId     int
	localDead      bool
	suspended      bool
	localUnitCount int
	localRuw       ReportableWorker
	remoteRuw      ReportableWorker
	remoteRrw      *remoteRelationsWorker

	applicationToken   string // token for app in local model
	relationToken      string // token for relation in local model
	localEndpoint      params.RemoteEndpoint
	remoteEndpointName string
	macaroon           *macaroon.Macaroon
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

func (w *remoteApplicationWorker) checkOfferPermissionDenied(ctx context.Context, err error, appToken, relationToken string) {
	// If consume permission has been revoked for the offer, set the
	// status of the local remote application entity.
	if params.ErrCode(err) == params.CodeDischargeRequired {
		if err := w.localModelFacade.SetRemoteApplicationStatus(ctx, w.applicationName, status.Error, err.Error()); err != nil {
			w.logger.Errorf(ctx,
				"updating remote application %v status from remote model %v: %v",
				w.applicationName, w.remoteModelUUID, err)
		}
		w.logger.Debugf(ctx, "discharge required error: app token: %v rel token: %v", appToken, relationToken)
		// If we know a specific relation, update that too.
		if relationToken != "" {
			suspended := true
			event := params.RemoteRelationChangeEvent{
				RelationToken:           relationToken,
				ApplicationOrOfferToken: appToken,
				Suspended:               &suspended,
				SuspendedReason:         "offer permission revoked",
			}
			if err := w.localModelFacade.ConsumeRemoteRelationChange(ctx, event); err != nil {
				w.logger.Errorf(ctx, "updating relation status: %v", err)
			}
		}
	}
}

func (w *remoteApplicationWorker) remoteOfferRemoved(ctx context.Context) error {
	w.logger.Debugf(ctx, "remote offer for %s has been removed", w.applicationName)
	if err := w.localModelFacade.SetRemoteApplicationStatus(ctx, w.applicationName, status.Terminated, "offer has been removed"); err != nil {
		return errors.Annotatef(err, "updating remote application %v status from remote model %v", w.applicationName, w.remoteModelUUID)
	}
	return nil
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

func (w *remoteApplicationWorker) loop() (err error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	// Watch for changes to any local relations to the remote application.
	relationsWatcher, err := w.localModelFacade.WatchRemoteApplicationRelations(ctx, w.applicationName)
	if err != nil && isNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "watching relations for remote application %q", w.applicationName)
	}
	if err := w.catacomb.Add(relationsWatcher); err != nil {
		return errors.Trace(err)
	}

	// On the consuming side, watch for status changes to the offer.
	var (
		offerStatusWatcher watcher.OfferStatusWatcher
		offerStatusChanges watcher.OfferStatusChannel
	)
	if !w.isConsumerProxy {
		if err := w.newRemoteRelationsFacadeWithRedirect(ctx); err != nil {
			msg := fmt.Sprintf("cannot connect to external controller: %v", err.Error())
			if err := w.localModelFacade.SetRemoteApplicationStatus(ctx, w.applicationName, status.Error, msg); err != nil {
				return errors.Annotatef(err, "updating remote application %v status from remote model %v", w.applicationName, w.remoteModelUUID)
			}
			return errors.Annotate(err, "cannot connect to external controller")
		}
		defer func() {
			if err := w.remoteModelFacade.Close(); err != nil {
				w.logger.Errorf(ctx, "error closing remote-relations facade: %s", err)
			}
		}()

		arg := params.OfferArg{
			OfferUUID: w.offerUUID,
		}
		if w.offerMacaroon != nil {
			arg.Macaroons = macaroon.Slice{w.offerMacaroon}
			arg.BakeryVersion = bakery.LatestVersion
		}

		offerStatusWatcher, err = w.remoteModelFacade.WatchOfferStatus(ctx, arg)
		if err != nil {
			w.checkOfferPermissionDenied(ctx, err, "", "")
			if isNotFound(err) {
				return w.remoteOfferRemoved(ctx)
			}
			return errors.Annotate(err, "watching status for offer")
		}
		if err := w.catacomb.Add(offerStatusWatcher); err != nil {
			return errors.Trace(err)
		}
		offerStatusChanges = offerStatusWatcher.Changes()
	}

	w.mu.Lock()
	w.relations = make(map[string]*relation)
	w.mu.Unlock()
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-relationsWatcher.Changes():
			w.logger.Debugf(ctx, "relations changed: %#v, %v", &change, ok)
			if !ok {
				// We are dying.
				return w.catacomb.ErrDying()
			}
			results, err := w.localModelFacade.Relations(ctx, change)
			if err != nil {
				return errors.Annotate(err, "querying relations")
			}
			for i, result := range results {
				key := change[i]
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
			w.logger.Debugf(ctx, "local relation units changed -> publishing: %#v", &change)
			// TODO(babbageclunk): add macaroons to event here instead
			// of in the relation units worker.
			if err := w.remoteModelFacade.PublishRelationChange(ctx, change.RemoteRelationChangeEvent); err != nil {
				w.checkOfferPermissionDenied(ctx, err, change.ApplicationOrOfferToken, change.RelationToken)
				if isNotFound(err) || params.IsCodeCannotEnterScope(err) {
					w.logger.Debugf(ctx, "relation %v changed but remote side already removed", change.Tag.Id())
					continue
				}
				return errors.Annotatef(err, "publishing relation change %#v to remote model %v", &change, w.remoteModelUUID)
			}

			// TODO(juju4) - remove
			// UnitCount has had omitempty removed, but we need to account for older controllers.
			zero := 0
			unitCount := change.UnitCount
			if unitCount == nil {
				unitCount = &zero
			}

			if err := w.localRelationChanged(ctx, change.Tag.Id(), unitCount); err != nil {
				return errors.Annotatef(err, "processing local relation change for %v", change.Tag.Id())
			}
		case change := <-w.remoteRelationUnitChanges:
			w.logger.Debugf(ctx, "remote relation units changed -> consuming: %#v", &change)
			if err := w.localModelFacade.ConsumeRemoteRelationChange(ctx, change.RemoteRelationChangeEvent); err != nil {
				if isNotFound(err) || params.IsCodeCannotEnterScope(err) {
					w.logger.Debugf(ctx, "relation %v changed but local side already removed", change.Tag.Id())
					continue
				}
				return errors.Annotatef(err, "consuming relation change %#v from remote model %v", &change, w.remoteModelUUID)
			}
		case changes := <-offerStatusChanges:
			w.logger.Debugf(ctx, "offer status changed: %#v", changes)
			for _, change := range changes {
				if err := w.localModelFacade.SetRemoteApplicationStatus(ctx, w.applicationName, change.Status.Status, change.Status.Message); err != nil {
					return errors.Annotatef(err, "updating remote application %v status from remote model %v", w.applicationName, w.remoteModelUUID)
				}
				// If the offer is terminated the status watcher can be stopped immediately.
				if change.Status.Status == status.Terminated {
					offerStatusWatcher.Kill()
					if err := offerStatusWatcher.Wait(); err != nil {
						w.logger.Warningf(ctx, "error stopping status watcher for saas application %s: %v", w.applicationName, err)
					}
					offerStatusChanges = nil
					break
				}
			}
		case changes := <-w.secretChanges:
			err := w.localModelFacade.ConsumeRemoteSecretChanges(ctx, changes)
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

// newRemoteRelationsFacadeWithRedirect attempts to open an API connection to
// the remote model for the watcher's application.
// If a redirect error is returned, we attempt to open a connection to the new
// controller and update our local controller info for the model so that future
// API connections are to the new location.
func (w *remoteApplicationWorker) newRemoteRelationsFacadeWithRedirect(ctx context.Context) error {
	apiInfo, err := w.localModelFacade.ControllerAPIInfoForModel(ctx, w.remoteModelUUID)
	if err != nil {
		return errors.Annotate(err, "cannot get controller api info for remote model")
	}
	w.logger.Debugf(ctx, "remote controller API addresses: %v", apiInfo.Addrs)

	w.remoteModelFacade, err = w.newRemoteModelRelationsFacadeFunc(ctx, apiInfo)
	var redirectErr *api.RedirectError
	if errors.As(errors.Cause(err), &redirectErr) {
		apiInfo.Addrs = network.CollapseToHostPorts(redirectErr.Servers).Strings()
		apiInfo.CACert = redirectErr.CACert

		w.logger.Debugf(ctx, "received redirect; new API addresses: %v", apiInfo.Addrs)

		if w.remoteModelFacade, err = w.newRemoteModelRelationsFacadeFunc(ctx, apiInfo); err == nil {
			// We successfully followed the redirect,
			// so update the controller information for this model.
			controllerInfo := crossmodel.ControllerInfo{
				ControllerUUID: redirectErr.ControllerTag.Id(),
				Alias:          redirectErr.ControllerAlias,
				Addrs:          apiInfo.Addrs,
				CACert:         apiInfo.CACert,
			}

			if err = w.localModelFacade.UpdateControllerForModel(ctx, controllerInfo, w.remoteModelUUID); err != nil {
				_ = w.remoteModelFacade.Close()
				err = errors.Annotate(err, "updating external controller info")
			}
		}
	}

	return errors.Annotate(err, "opening facade to remote model")
}

func (w *remoteApplicationWorker) processRelationDying(ctx context.Context, key string, r *relation, forceCleanup bool) error {
	w.logger.Debugf(ctx, "relation %v dying (%v)", key, forceCleanup)
	// On the consuming side, inform the remote side the relation is dying
	// (but only if we are killing the relation due to it dying, not because
	// it is suspended).
	if !w.isConsumerProxy {
		change := params.RemoteRelationChangeEvent{
			RelationToken:           r.relationToken,
			Life:                    life.Dying,
			ApplicationOrOfferToken: r.applicationToken,
			Macaroons:               macaroon.Slice{r.macaroon},
			BakeryVersion:           bakery.LatestVersion,
		}
		// forceCleanup will be true if the worker has restarted and because the relation had
		// already been removed, we won't get any more unit departed events.
		if forceCleanup {
			change.ForceCleanup = &forceCleanup
		}
		if err := w.remoteModelFacade.PublishRelationChange(ctx, change); err != nil {
			w.checkOfferPermissionDenied(ctx, err, r.applicationToken, r.relationToken)
			if isNotFound(err) {
				w.logger.Debugf(ctx, "relation %v dying but remote side already removed", key)
				return nil
			}
			return errors.Annotatef(err, "publishing relation dying %#v to remote model %v", &change, w.remoteModelUUID)
		}
	}
	return nil
}

func (w *remoteApplicationWorker) processRelationSuspended(ctx context.Context, key string, relLife life.Value, relations map[string]*relation) error {
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

	// On the offering side, if the relation is resumed,
	// it will be treated like the relation has been joined
	// for the first time; all workers will be restarted.
	// The offering side has isConsumerProxy = true.
	if w.isConsumerProxy {
		delete(relations, key)
	}

	if relation.localRuw != nil {
		if err := worker.Stop(relation.localRuw); err != nil {
			w.logger.Warningf(ctx, "stopping local relation unit worker for %v: %v", key, err)
		}
		relation.localRuw = nil
	}
	if relation.remoteRuw != nil {
		if err := worker.Stop(relation.remoteRuw); err != nil {
			w.logger.Warningf(ctx, "stopping remote relation unit worker for %v: %v", key, err)
		}
		relation.remoteRuw = nil
	}
	return nil
}

// processLocalRelationRemoved is called when a change event arrives from the remote model
// but the relation in the local model has been removed.
func (w *remoteApplicationWorker) processLocalRelationRemoved(ctx context.Context, key string, relations map[string]*relation) error {
	w.logger.Debugf(ctx, "local relation %v removed", key)
	relation, ok := relations[key]
	if !ok {
		return nil
	}

	// Stop the worker which watches remote status/life.
	if relation.remoteRrw != nil {
		if err := worker.Stop(relation.remoteRrw); err != nil {
			w.logger.Warningf(ctx, "stopping remote relations worker for %v: %v", key, err)
		}
		relation.remoteRrw = nil
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
	if relation.localRuw != nil {
		if err := worker.Stop(relation.localRuw); err != nil {
			w.logger.Warningf(ctx, "stopping local relation unit worker for %v: %v", key, err)
		}
		relation.localRuw = nil
	}
	if relation.remoteRuw != nil {
		if err := worker.Stop(relation.remoteRuw); err != nil {
			w.logger.Warningf(ctx, "stopping remote relation unit worker for %v: %v", key, err)
		}
		relation.remoteRuw = nil
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

	if w.isConsumerProxy {
		// Nothing else to do on the offering side.
		return nil
	}
	return w.processConsumingRelation(ctx, key, localRelationInfo)
}

// startUnitsWorkers starts 2 workers to watch for unit settings or departed changes;
// one worker is for the local model, the other for the remote model.
func (w *remoteApplicationWorker) startUnitsWorkers(
	ctx context.Context,
	relationTag names.RelationTag,
	relationToken, remoteAppToken string,
	applicationName string,
	mac *macaroon.Macaroon,
) (ReportableWorker, ReportableWorker, error) {
	localUnitsWorker, err := newLocalRelationUnitsWorker(
		ctx,
		w.localModelFacade,
		relationTag,
		mac,
		w.localRelationUnitChanges,
		w.logger,
	)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if err := w.catacomb.Add(localUnitsWorker); err != nil {
		return nil, nil, errors.Trace(err)
	}

	remoteUnitsWorker, err := newRemoteRelationUnitsWorker(
		ctx,
		w.remoteModelFacade,
		relationTag,
		mac,
		relationToken, remoteAppToken, applicationName,
		w.remoteRelationUnitChanges,
		w.logger,
	)
	if err != nil {
		w.checkOfferPermissionDenied(ctx, err, remoteAppToken, relationToken)
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
	applicationToken, remoteAppToken, relationToken, mac, err := w.registerRemoteRelation(
		ctx,
		applicationTag, relationTag, w.offerUUID, w.consumeVersion,
		remoteRelation.Endpoint, remoteRelation.RemoteEndpointName)
	if err != nil {
		w.checkOfferPermissionDenied(ctx, err, "", "")
		return errors.Annotatef(err, "registering application %v and relation %v", remoteRelation.ApplicationName, relationTag.Id())
	}
	w.logger.Debugf(ctx, "remote relation registered for %q: app token=%q, rel token=%q, remote app token=%q", key, applicationToken, relationToken, remoteAppToken)

	// Have we seen the relation before.
	r, relationKnown := w.relations[key]
	if !relationKnown {
		// Totally new so start the lifecycle watcher.
		remoteRelationsWatcher, err := w.remoteModelFacade.WatchRelationSuspendedStatus(ctx, params.RemoteEntityArg{
			Token:         relationToken,
			Macaroons:     macaroon.Slice{mac},
			BakeryVersion: bakery.LatestVersion,
		})
		if err != nil {
			w.checkOfferPermissionDenied(ctx, err, remoteAppToken, relationToken)
			return errors.Annotatef(err, "watching remote side of relation %v", remoteRelation.Key)
		}

		remoteRelationsWorker, err := newRemoteRelationsWorker(
			relationTag,
			remoteAppToken,
			relationToken,
			remoteRelationsWatcher,
			w.remoteRelationUnitChanges,
			w.logger,
		)
		if err != nil {
			return errors.Trace(err)
		}
		if err := w.catacomb.Add(remoteRelationsWorker); err != nil {
			return errors.Trace(err)
		}
		r = &relation{
			relationId:         remoteRelation.Id,
			suspended:          remoteRelation.Suspended,
			localUnitCount:     remoteRelation.UnitCount,
			remoteRrw:          remoteRelationsWorker,
			macaroon:           mac,
			localEndpoint:      remoteRelation.Endpoint,
			remoteEndpointName: remoteRelation.RemoteEndpointName,
			applicationToken:   applicationToken,
			relationToken:      relationToken,
		}
		w.relations[key] = r
	}

	if r.localRuw == nil && !remoteRelation.Suspended {
		// Also start the units watchers (local and remote).
		localUnitsWorker, remoteUnitsWorker, err := w.startUnitsWorkers(
			ctx,
			relationTag, relationToken, remoteAppToken, remoteRelation.ApplicationName,
			mac)
		if err != nil {
			return errors.Annotate(err, "starting relation units workers")
		}
		r.localRuw = localUnitsWorker
		r.remoteRuw = remoteUnitsWorker
	}

	if w.secretChangesWatcher == nil {
		w.secretChangesWatcher, err = w.remoteModelFacade.WatchConsumedSecretsChanges(ctx, applicationToken, relationToken, w.offerMacaroon)
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
) (applicationToken, offeringAppToken, relationToken string, _ *macaroon.Macaroon, _ error) {
	w.logger.Debugf(ctx, "register remote relation %v to local application %v", relationTag.Id(), applicationTag.Id())

	fail := func(err error) (string, string, string, *macaroon.Macaroon, error) {
		return "", "", "", nil, err
	}

	// Ensure the relation is exported first up.
	results, err := w.localModelFacade.ExportEntities(ctx, []names.Tag{applicationTag, relationTag})
	if err != nil {
		return fail(errors.Annotatef(err, "exporting relation %v and application %v", relationTag, applicationTag))
	}
	if results[0].Error != nil && !params.IsCodeAlreadyExists(results[0].Error) {
		return fail(errors.Annotatef(err, "exporting application %v", applicationTag))
	}
	applicationToken = results[0].Token
	if results[1].Error != nil && !params.IsCodeAlreadyExists(results[1].Error) {
		return fail(errors.Annotatef(err, "exporting relation %v", relationTag))
	}
	relationToken = results[1].Token

	// This data goes to the remote model so we map local info
	// from this model to the remote arg values and visa versa.
	arg := params.RegisterRemoteRelationArg{
		ApplicationToken:  applicationToken,
		SourceModelTag:    names.NewModelTag(w.localModelUUID).String(),
		RelationToken:     relationToken,
		OfferUUID:         offerUUID,
		RemoteEndpoint:    localEndpointInfo,
		LocalEndpointName: remoteEndpointName,
		ConsumeVersion:    consumeVersion,
	}
	if w.offerMacaroon != nil {
		arg.Macaroons = macaroon.Slice{w.offerMacaroon}
		arg.BakeryVersion = bakery.LatestVersion
	}
	remoteRelation, err := w.remoteModelFacade.RegisterRemoteRelations(ctx, arg)
	if err != nil {
		return fail(errors.Trace(err))
	}
	// remoteAppIds is a slice but there's only one item
	// as we currently only register one remote application
	if err := remoteRelation[0].Error; err != nil {
		return fail(errors.Annotatef(err, "registering relation %v", relationTag))
	}
	// Import the application id from the offering model.
	registerResult := *remoteRelation[0].Result
	offeringAppToken = registerResult.Token
	// We have a new macaroon attenuated to the relation.
	// Save for the firewaller.
	if err := w.localModelFacade.SaveMacaroon(ctx, relationTag, registerResult.Macaroon); err != nil {
		return fail(errors.Annotatef(
			err, "saving macaroon for %v", relationTag))
	}

	appTag := names.NewApplicationTag(w.applicationName)
	w.logger.Debugf(ctx, "import remote application token %v for %v", offeringAppToken, w.applicationName)
	err = w.localModelFacade.ImportRemoteEntity(ctx, appTag, offeringAppToken)
	if err != nil && !params.IsCodeAlreadyExists(err) {
		return fail(errors.Annotatef(
			err, "importing remote application %v to local model", w.applicationName))
	}
	return applicationToken, offeringAppToken, relationToken, registerResult.Macaroon, nil
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
			"application-token": info.applicationToken,
			"relation-token":    info.relationToken,
			"local-endpoint":    info.localEndpoint.Name,
			"remote-endpoint":   info.remoteEndpointName,
		}
		if info.remoteRrw != nil {
			report["last-status-event"] = info.remoteRrw.Report()
		}
		if info.localRuw != nil {
			report["last-local-change"] = info.localRuw.Report()
		}
		if info.remoteRuw != nil {
			report["last-remote-change"] = info.remoteRuw.Report()
		}
		relationsInfo[rel] = report
	}
	if len(relationsInfo) > 0 {
		result["relations"] = relationsInfo
	}
	result["remote-model-uuid"] = w.remoteModelUUID
	if w.isConsumerProxy {
		result["consumer-proxy"] = true
		result["consume-version"] = w.consumeVersion
	} else {
		result["saas-application"] = true
		result["offer-uuid"] = w.offerUUID
	}

	return result
}

func (w *remoteApplicationWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
