// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

// remoteApplicationWorker listens for localChanges to relations
// involving a remote application, and publishes change to
// local relation units to the remote model. It also watches for
// changes originating from the offering model and consumes those
// in the local model.
type remoteApplicationWorker struct {
	catacomb         catacomb.Catacomb
	relationsWatcher watcher.StringsWatcher

	// These attribute are relevant to dealing with a specific
	// remote application proxy.
	// TODO(wallyworld) - change to offer UUID
	offerName             string
	applicationName       string // name of the remote application proxy in the local model
	localModelUUID        string // uuid of the model hosting the local application
	remoteModelUUID       string // uuid of the model hosting the remote offer
	registered            bool
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
	life       params.Life
	localRuw   *relationUnitsWorker
	remoteRuw  *relationUnitsWorker
	remoteRrw  *remoteRelationsWorker

	applicationToken   string // token for app in local model
	localEndpoint      params.RemoteEndpoint
	remoteEndpointName string
	macaroon           *macaroon.Macaroon
}

func newRemoteApplicationWorker(
	relationsWatcher watcher.StringsWatcher,
	localModelUUID string,
	remoteApplication params.RemoteApplication,
	newRemoteModelRelationsFacadeFunc newRemoteRelationsFacadeFunc,
	facade RemoteRelationsFacade,
) (worker.Worker, error) {
	w := &remoteApplicationWorker{
		relationsWatcher:                  relationsWatcher,
		offerName:                         remoteApplication.OfferName,
		applicationName:                   remoteApplication.Name,
		localModelUUID:                    localModelUUID,
		remoteModelUUID:                   remoteApplication.ModelUUID,
		registered:                        remoteApplication.Registered,
		offerMacaroon:                     remoteApplication.Macaroon,
		localRelationChanges:              make(chan params.RemoteRelationChangeEvent),
		remoteRelationChanges:             make(chan params.RemoteRelationChangeEvent),
		localModelFacade:                  facade,
		newRemoteModelRelationsFacadeFunc: newRemoteModelRelationsFacadeFunc,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{relationsWatcher},
	})
	return w, err
}

// Kill is defined on worker.Worker
func (w *remoteApplicationWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *remoteApplicationWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *remoteApplicationWorker) loop() error {
	defer func() {
		if w.remoteModelFacade != nil {
			w.remoteModelFacade.Close()
		}
	}()

	relations := make(map[string]*relation)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-w.relationsWatcher.Changes():
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
				return errors.Annotatef(err, "publishing relation change %+v to remote model %v", change, w.remoteModelUUID)
			}
		case change := <-w.remoteRelationChanges:
			logger.Debugf("remote relation units changed -> consuming: %#v", change)
			if err := w.localModelFacade.ConsumeRemoteRelationChange(change); err != nil {
				return errors.Annotatef(err, "consuming relation change %+v from remote model %v", change, w.remoteModelUUID)
			}
		}
	}
}

func (w *remoteApplicationWorker) processRelationGone(key string, relations map[string]*relation) error {
	logger.Debugf("relation %v gone", key)
	relation, ok := relations[key]
	if !ok {
		return nil
	}
	delete(relations, key)
	if err := worker.Stop(relation.localRuw); err != nil {
		logger.Warningf("stopping local relation unit worker for %v: %v", key, err)
	}
	if err := worker.Stop(relation.remoteRuw); err != nil {
		logger.Warningf("stopping remote relation unit worker for %v: %v", key, err)
	}
	if err := worker.Stop(relation.remoteRrw); err != nil {
		logger.Warningf("stopping remote relations worker for %v: %v", key, err)
	}

	// Remove the remote entity record for the relation to ensure any unregister
	// call from the remote model that may come across at the same time is short circuited.
	remoteId := relation.localRuw.remoteRelationToken
	relTag := names.NewRelationTag(key)
	_, err := w.localModelFacade.GetToken(relTag)
	if errors.IsNotFound(err) {
		logger.Debugf("not found token for %v in %v, exit early", key, w.localModelUUID)
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// On the consuming side, inform the remote side the relation is dying.
	if !w.registered {
		change := params.RemoteRelationChangeEvent{
			RelationToken:    remoteId,
			Life:             params.Dying,
			ApplicationToken: relation.applicationToken,
			Macaroons:        macaroon.Slice{relation.macaroon},
		}
		if err := w.remoteModelFacade.PublishRelationChange(change); err != nil {
			return errors.Annotatef(err, "publishing relation departed %+v to remote model %v", change, w.remoteModelUUID)
		}
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
			return w.processRelationGone(key, relations)
		}
		return result.Error
	}
	remoteRelation := result.Result

	// If we have previously started the watcher and the
	// relation is now dying, stop the watcher.
	if r := relations[key]; r != nil {
		r.life = remoteRelation.Life
		if r.life == params.Dying {
			return w.processRelationGone(key, relations)
		}
		// Nothing to do, we have previously started the watcher.
		return nil
	}
	if remoteRelation.Life != params.Alive {
		// We haven't started the relation unit watcher so just exit.
		return nil
	}
	if w.registered {
		// Nothing else to do on the offering side.
		return nil
	}
	return w.processNewConsumingRelation(key, relations, remoteRelation)
}

// processNewConsumingRelation starts the sub-workers necessary to listen and publish
// local unit settings changes, and watch and consume remote unit settings changes.
func (w *remoteApplicationWorker) processNewConsumingRelation(
	key string,
	relations map[string]*relation,
	remoteRelation *params.RemoteRelation,
) error {
	// Get the connection info for the remote controller.
	apiInfo, err := w.localModelFacade.ControllerAPIInfoForModel(w.remoteModelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	w.remoteModelFacade, err = w.newRemoteModelRelationsFacadeFunc(apiInfo)
	if err != nil {
		return errors.Annotate(err, "opening facade to remote model")
	}

	// We have not seen the relation before, make
	// sure it is registered on the offering side.
	applicationTag := names.NewApplicationTag(remoteRelation.ApplicationName)
	relationTag := names.NewRelationTag(key)
	applicationToken, remoteAppToken, relationToken, mac, err := w.registerRemoteRelation(
		applicationTag, relationTag, w.offerName,
		remoteRelation.Endpoint, remoteRelation.RemoteEndpointName)
	if err != nil {
		return errors.Annotatef(err, "registering application %v and relation %v", remoteRelation.ApplicationName, relationTag.Id())
	}

	// Start a watcher to track changes to the units in the relation in the local model.
	localRelationUnitsWatcher, err := w.localModelFacade.WatchLocalRelationUnits(key)
	if err != nil {
		return errors.Annotatef(err, "watching local side of relation %v", relationTag.Id())
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
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(localUnitsWorker); err != nil {
		return errors.Trace(err)
	}

	// Start a watcher to track changes to the units in the relation in the remote model.
	remoteRelationUnitsWatcher, err := w.remoteModelFacade.WatchRelationUnits(params.RemoteEntityArg{
		Token:     relationToken,
		Macaroons: macaroon.Slice{mac},
	})
	if err != nil {
		return errors.Annotatef(
			err, "watching remote side of application %v and relation %v",
			remoteRelation.ApplicationName, relationTag.Id())
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
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(remoteUnitsWorker); err != nil {
		return errors.Trace(err)
	}

	remoteRelationsWatcher, err := w.remoteModelFacade.WatchRelationStatus(params.RemoteEntityArg{
		Token:     relationToken,
		Macaroons: macaroon.Slice{mac},
	})
	if err != nil {
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

	relations[key] = &relation{
		relationId:         remoteRelation.Id,
		life:               remoteRelation.Life,
		localRuw:           localUnitsWorker,
		remoteRuw:          remoteUnitsWorker,
		remoteRrw:          remoteRelationsWorker,
		macaroon:           mac,
		localEndpoint:      remoteRelation.Endpoint,
		remoteEndpointName: remoteRelation.RemoteEndpointName,
		applicationToken:   applicationToken,
	}

	return nil
}

func (w *remoteApplicationWorker) registerRemoteRelation(
	applicationTag, relationTag names.Tag,
	remoteApplicationOfferName string,
	localEndpointInfo params.RemoteEndpoint, remoteEndpointName string,
) (applicationToken, offeringAppToken, relationToken string, _ *macaroon.Macaroon, _ error) {
	logger.Debugf("register remote relation %v", relationTag.Id())

	fail := func(err error) (string, string, string, *macaroon.Macaroon, error) {
		return "", "", "", nil, err
	}

	// Ensure the relation is exported first up.
	results, err := w.localModelFacade.ExportEntities([]names.Tag{applicationTag, relationTag})
	if err != nil {
		return fail(errors.Annotatef(err, "exporting relation %v and application", relationTag, applicationTag))
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
		OfferName:         remoteApplicationOfferName,
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
		return fail(errors.Trace(err))
	}
	if err := results[0].Error; err != nil && !params.IsCodeAlreadyExists(err) {
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
