// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

// remoteApplicationWorker listens for localChanges to relations
// involving a remote application, and publishes change to
// local relation units to the remote model. It also watches for
// changes originating from the offering model and consumes those
// in the local model.
type remoteApplicationWorker struct {
	catacomb catacomb.Catacomb

	// These attribute are relevant to dealing with a specific
	// remote application proxy.
	offerUUID             string
	applicationName       string // name of the remote application proxy in the local model
	localModelUUID        string // uuid of the model hosting the local application
	remoteModelUUID       string // uuid of the model hosting the remote offer
	isConsumerProxy       bool
	localRelationChanges  chan params.RemoteRelationChangeEvent
	remoteRelationChanges chan params.RemoteRelationChangeEvent

	// offerMacaroon is used to confirm that permission has been granted to consume
	// the remote application to which this worker pertains.
	offerMacaroon *macaroon.Macaroon

	// localModelFacade interacts with the local (consuming) model.
	localModelFacade RemoteRelationsFacade
	// remoteModelFacade interacts with the remote (offering) model.
	remoteModelFacade RemoteModelRelationsFacadeCloser

	newRemoteModelRelationsFacadeFunc newRemoteRelationsFacadeFunc
}

// relation holds attributes relevant to a particular
// relation between a local app and a remote offer.
type relation struct {
	relationId int
	suspended  bool
	localRuw   *relationUnitsWorker
	remoteRuw  *relationUnitsWorker
	remoteRrw  *remoteRelationsWorker

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
		logger.Errorf("error in remote application worker for %v: %v", w.applicationName, err)
	}
	return err
}

func (w *remoteApplicationWorker) checkOfferPermissionDenied(err error, appToken, relationToken string) {
	// If consume permission has been revoked for the offer, set the
	// status of the local remote application entity.
	if params.ErrCode(err) == params.CodeDischargeRequired {
		if err := w.localModelFacade.SetRemoteApplicationStatus(w.applicationName, status.Error, err.Error()); err != nil {
			logger.Errorf(
				"updating remote application %v status from remote model %v: %v",
				w.applicationName, w.remoteModelUUID, err)
		}
		// If we know a specific relation, update that too.
		if relationToken != "" {
			suspended := true
			event := params.RemoteRelationChangeEvent{
				RelationToken:    relationToken,
				ApplicationToken: appToken,
				Suspended:        &suspended,
				SuspendedReason:  "offer permission revoked",
			}
			if err := w.localModelFacade.ConsumeRemoteRelationChange(event); err != nil {
				logger.Errorf("updating relation status: %v", err)
			}
		}
	}
}

func (w *remoteApplicationWorker) loop() (err error) {
	// Watch for changes to any remote relations to this application.
	relationsWatcher, err := w.localModelFacade.WatchRemoteApplicationRelations(w.applicationName)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "watching relations for remote application %q", w.applicationName)
	}
	if err := w.catacomb.Add(relationsWatcher); err != nil {
		return errors.Trace(err)
	}

	// On the consuming side, watch for status changes to the offer.
	var offerStatusChanges watcher.OfferStatusChannel
	if !w.isConsumerProxy {
		// Get the connection info for the remote controller.
		apiInfo, err := w.localModelFacade.ControllerAPIInfoForModel(w.remoteModelUUID)
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("remote controller api addresses: %v", apiInfo.Addrs)

		w.remoteModelFacade, err = w.newRemoteModelRelationsFacadeFunc(apiInfo)
		if err != nil {
			return errors.Annotate(err, "opening facade to remote model")
		}

		defer func() {
			w.remoteModelFacade.Close()
		}()

		arg := params.OfferArg{
			OfferUUID: w.offerUUID,
		}
		if w.offerMacaroon != nil {
			arg.Macaroons = macaroon.Slice{w.offerMacaroon}
		}

		offerStatusWatcher, err := w.remoteModelFacade.WatchOfferStatus(arg)
		if err != nil {
			w.checkOfferPermissionDenied(err, "", "")
			return errors.Annotate(err, "watching status for offer")
		}
		if err := w.catacomb.Add(offerStatusWatcher); err != nil {
			return errors.Trace(err)
		}
		offerStatusChanges = offerStatusWatcher.Changes()
	}

	relations := make(map[string]*relation)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-relationsWatcher.Changes():
			logger.Debugf("relations changed: %#v, %v", change, ok)
			if !ok {
				// We are dying.
				return w.catacomb.ErrDying()
			}
			results, err := w.localModelFacade.Relations(change)
			if err != nil {
				return errors.Annotate(err, "querying relations")
			}
			for i, result := range results {
				key := change[i]
				if err := w.relationChanged(key, result, relations); err != nil {
					return errors.Annotatef(err, "handling change for relation %q", key)
				}
			}
		case change := <-w.localRelationChanges:
			logger.Debugf("local relation units changed -> publishing: %#v", change)
			if err := w.remoteModelFacade.PublishRelationChange(change); err != nil {
				w.checkOfferPermissionDenied(err, change.ApplicationToken, change.RelationToken)
				return errors.Annotatef(err, "publishing relation change %+v to remote model %v", change, w.remoteModelUUID)
			}
		case change := <-w.remoteRelationChanges:
			logger.Debugf("remote relation units changed -> consuming: %#v", change)
			if err := w.localModelFacade.ConsumeRemoteRelationChange(change); err != nil {
				return errors.Annotatef(err, "consuming relation change %+v from remote model %v", change, w.remoteModelUUID)
			}
		case changes := <-offerStatusChanges:
			logger.Debugf("offer status changed: %#v", changes)
			for _, change := range changes {
				if err := w.localModelFacade.SetRemoteApplicationStatus(w.applicationName, change.Status.Status, change.Status.Message); err != nil {
					return errors.Annotatef(err, "updating remote application %v status from remote model %v", w.applicationName, w.remoteModelUUID)
				}
			}
		}
	}
}

func (w *remoteApplicationWorker) processRelationDying(key string, r *relation, forceCleanup bool) error {
	logger.Debugf("relation %v dying (%v)", key, forceCleanup)
	// On the consuming side, inform the remote side the relation is dying
	// (but only if we are killing the relation due to it dying, not because
	// it is suspended).
	if !w.isConsumerProxy {
		change := params.RemoteRelationChangeEvent{
			RelationToken:    r.relationToken,
			Life:             params.Dying,
			ApplicationToken: r.applicationToken,
			Macaroons:        macaroon.Slice{r.macaroon},
		}
		// forceCleanup will be true if the worker has restarted and because the relation had
		// already been removed, we won't get any more unit departed events.
		if forceCleanup {
			change.ForceCleanup = &forceCleanup
		}
		if err := w.remoteModelFacade.PublishRelationChange(change); err != nil {
			w.checkOfferPermissionDenied(err, r.applicationToken, r.relationToken)
			return errors.Annotatef(err, "publishing relation dying %+v to remote model %v", change, w.remoteModelUUID)
		}
	}
	return nil
}

func (w *remoteApplicationWorker) processRelationSuspended(key string, relations map[string]*relation) error {
	logger.Debugf("relation %v suspended", key)
	relation, ok := relations[key]
	if !ok {
		return nil
	}

	// For suspended relations on the consuming side
	// we want to keep the remote lifecycle watcher
	// so we know when the relation is resumed.
	if w.isConsumerProxy {
		if err := worker.Stop(relation.remoteRrw); err != nil {
			logger.Warningf("stopping remote relations worker for %v: %v", key, err)
		}
		relation.remoteRuw = nil
		delete(relations, key)
	}

	if relation.localRuw != nil {
		if err := worker.Stop(relation.localRuw); err != nil {
			logger.Warningf("stopping local relation unit worker for %v: %v", key, err)
		}
		relation.localRuw = nil
	}
	return nil
}

func (w *remoteApplicationWorker) processRelationRemoved(key string, relations map[string]*relation) error {
	logger.Debugf("relation %v removed", key)
	relation, ok := relations[key]
	if !ok {
		return nil
	}

	if err := worker.Stop(relation.remoteRrw); err != nil {
		logger.Warningf("stopping remote relations worker for %v: %v", key, err)
	}
	relation.remoteRuw = nil
	delete(relations, key)

	// For the unit watchers, check to see if these are nil before stopping.
	// They will be nil if the relation was suspended and then we kill it for real.
	if relation.localRuw != nil {
		if err := worker.Stop(relation.localRuw); err != nil {
			logger.Warningf("stopping local relation unit worker for %v: %v", key, err)
		}
		relation.localRuw = nil
	}

	logger.Debugf("remote relation %v removed from remote model", key)
	return nil
}

func (w *remoteApplicationWorker) relationChanged(
	key string, result params.RemoteRelationResult, relations map[string]*relation,
) error {
	logger.Debugf("relation %q changed: %+v", key, result)
	if result.Error != nil {
		if params.IsCodeNotFound(result.Error) {
			return w.processRelationRemoved(key, relations)
		}
		return result.Error
	}
	remoteRelation := result.Result

	// If we have previously started the watcher and the
	// relation is now suspended, stop the watcher.
	if r := relations[key]; r != nil {
		wasSuspended := r.suspended
		r.suspended = remoteRelation.Suspended
		relations[key] = r
		if remoteRelation.Suspended {
			return w.processRelationSuspended(key, relations)
		}
		if !wasSuspended && remoteRelation.Life == params.Alive {
			// Nothing to do, we have previously started the watcher.
			return nil
		}
	}

	if w.isConsumerProxy {
		// Nothing else to do on the offering side.
		return nil
	}
	return w.processConsumingRelation(key, relations, remoteRelation)
}

// startUnitsWorkers starts 2 workers to watch for unit settings or departed changes;
// one worker is for the local model, the other for the remote model.
func (w *remoteApplicationWorker) startUnitsWorkers(
	relationTag names.RelationTag,
	applicationToken, relationToken, remoteAppToken string,
	applicationName string,
	mac *macaroon.Macaroon,
) (*relationUnitsWorker, *relationUnitsWorker, error) {
	// Start a watcher to track changes to the units in the relation in the local model.
	localRelationUnitsWatcher, err := w.localModelFacade.WatchLocalRelationUnits(relationTag.Id())
	if err != nil {
		return nil, nil, errors.Annotatef(err, "watching local side of relation %v", relationTag.Id())
	}

	// localUnitSettingsFunc converts relations units watcher results from the local model
	// into settings params using an api call to the local model.
	localUnitSettingsFunc := func(changedUnitNames []string) ([]params.SettingsResult, error) {
		relationUnits := make([]params.RelationUnit, len(changedUnitNames))
		for i, changedName := range changedUnitNames {
			relationUnits[i] = params.RelationUnit{
				Relation: relationTag.String(),
				Unit:     names.NewUnitTag(changedName).String(),
			}
		}
		return w.localModelFacade.RelationUnitSettings(relationUnits)
	}
	localUnitsWorker, err := newRelationUnitsWorker(
		relationTag,
		applicationToken,
		mac,
		relationToken,
		localRelationUnitsWatcher,
		w.localRelationChanges,
		localUnitSettingsFunc,
	)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if err := w.catacomb.Add(localUnitsWorker); err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Start a watcher to track changes to the units in the relation in the remote model.
	remoteRelationUnitsWatcher, err := w.remoteModelFacade.WatchRelationUnits(params.RemoteEntityArg{
		Token:     relationToken,
		Macaroons: macaroon.Slice{mac},
	})
	if err != nil {
		w.checkOfferPermissionDenied(err, remoteAppToken, relationToken)
		return nil, nil, errors.Annotatef(
			err, "watching remote side of application %v and relation %v",
			applicationName, relationTag.Id())
	}

	// remoteUnitSettingsFunc converts relations units watcher results from the remote model
	// into settings params using an api call to the remote model.
	remoteUnitSettingsFunc := func(changedUnitNames []string) ([]params.SettingsResult, error) {
		relationUnits := make([]params.RemoteRelationUnit, len(changedUnitNames))
		for i, changedName := range changedUnitNames {
			relationUnits[i] = params.RemoteRelationUnit{
				RelationToken: relationToken,
				Unit:          names.NewUnitTag(changedName).String(),
				Macaroons:     macaroon.Slice{mac},
			}
		}
		return w.remoteModelFacade.RelationUnitSettings(relationUnits)
	}
	remoteUnitsWorker, err := newRelationUnitsWorker(
		relationTag,
		remoteAppToken,
		mac,
		relationToken,
		remoteRelationUnitsWatcher,
		w.remoteRelationChanges,
		remoteUnitSettingsFunc,
	)
	if err != nil {
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
	key string,
	relations map[string]*relation,
	remoteRelation *params.RemoteRelation,
) error {

	// We have not seen the relation before, make
	// sure it is registered on the offering side.
	// Or relation was suspended and is now resumed so re-register.
	applicationTag := names.NewApplicationTag(remoteRelation.ApplicationName)
	relationTag := names.NewRelationTag(key)
	applicationToken, remoteAppToken, relationToken, mac, err := w.registerRemoteRelation(
		applicationTag, relationTag, w.offerUUID,
		remoteRelation.Endpoint, remoteRelation.RemoteEndpointName)
	if err != nil {
		w.checkOfferPermissionDenied(err, "", "")
		return errors.Annotatef(err, "registering application %v and relation %v", remoteRelation.ApplicationName, relationTag.Id())
	}

	// Have we seen the relation before.
	r, relationKnown := relations[key]
	if !relationKnown {
		// Totally new so start the lifecycle watcher.
		remoteRelationsWatcher, err := w.remoteModelFacade.WatchRelationSuspendedStatus(params.RemoteEntityArg{
			Token:     relationToken,
			Macaroons: macaroon.Slice{mac},
		})
		if err != nil {
			w.checkOfferPermissionDenied(err, remoteAppToken, relationToken)
			return errors.Annotatef(err, "watching remote side of relation %v", remoteRelation.Key)
		}

		remoteRelationsWorker, err := newRemoteRelationsWorker(
			relationTag,
			remoteAppToken,
			relationToken,
			remoteRelationsWatcher,
			w.remoteRelationChanges,
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
			remoteRrw:          remoteRelationsWorker,
			macaroon:           mac,
			localEndpoint:      remoteRelation.Endpoint,
			remoteEndpointName: remoteRelation.RemoteEndpointName,
			applicationToken:   applicationToken,
			relationToken:      relationToken,
		}
		relations[key] = r
	}

	if r.localRuw == nil && !remoteRelation.Suspended {
		// Also start the units watchers (local and remote).
		localUnitsWorker, remoteUnitsWorker, err := w.startUnitsWorkers(
			relationTag, applicationToken, relationToken, remoteAppToken, remoteRelation.ApplicationName, mac)
		if err != nil {
			return errors.Annotate(err, "starting relation units workers")
		}
		r.localRuw = localUnitsWorker
		r.remoteRuw = remoteUnitsWorker
	}

	// If the relation is dying, stop the watcher.
	if remoteRelation.Life != params.Alive {
		return w.processRelationDying(key, r, !relationKnown)
	}

	return nil
}

func (w *remoteApplicationWorker) registerRemoteRelation(
	applicationTag, relationTag names.Tag, offerUUID string,
	localEndpointInfo params.RemoteEndpoint, remoteEndpointName string,
) (applicationToken, offeringAppToken, relationToken string, _ *macaroon.Macaroon, _ error) {
	logger.Debugf("register remote relation %v", relationTag.Id())

	fail := func(err error) (string, string, string, *macaroon.Macaroon, error) {
		return "", "", "", nil, err
	}

	// Ensure the relation is exported first up.
	results, err := w.localModelFacade.ExportEntities([]names.Tag{applicationTag, relationTag})
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
	}
	if w.offerMacaroon != nil {
		arg.Macaroons = macaroon.Slice{w.offerMacaroon}
	}
	remoteRelation, err := w.remoteModelFacade.RegisterRemoteRelations(arg)
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
	if err := w.localModelFacade.SaveMacaroon(relationTag, registerResult.Macaroon); err != nil {
		return fail(errors.Annotatef(
			err, "saving macaroon for %v", relationTag))
	}

	appTag := names.NewApplicationTag(w.applicationName)
	logger.Debugf("import remote application token %v for %v", offeringAppToken, w.applicationName)
	err = w.localModelFacade.ImportRemoteEntity(appTag, offeringAppToken)
	if err != nil && !params.IsCodeAlreadyExists(err) {
		return fail(errors.Annotatef(
			err, "importing remote application %v to local model", w.applicationName))
	}
	return applicationToken, offeringAppToken, relationToken, registerResult.Macaroon, nil
}
