// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"io"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.remoterelations")

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
	// RegisterRemoteRelation sets up the remote model to participate
	// in the specified relations.
	RegisterRemoteRelations(relations ...params.RegisterRemoteRelation) ([]params.RemoteEntityIdResult, error)

	// PublishRelationChange publishes relation changes to the
	// model hosting the remote application involved in the relation.
	PublishRelationChange(params.RemoteRelationChangeEvent) error

	// WatchRelationUnits returns a watcher that notifies of changes to the
	// units in the remote model for the relation with the given remote id.
	WatchRelationUnits(params.RemoteEntityId) (watcher.RelationUnitsWatcher, error)

	// RelationUnitSettings returns the relation unit settings for the given relation units in the remote model.
	RelationUnitSettings([]params.RemoteRelationUnit) ([]params.SettingsResult, error)
}

// RemoteRelationsFacade exposes remote relation functionality to a worker.
type RemoteRelationsFacade interface {
	// ImportRemoteEntity adds an entity to the remote entities collection
	// with the specified opaque token.
	ImportRemoteEntity(sourceModelUUID string, entity names.Tag, token string) error

	// ExportEntities allocates unique, remote entity IDs for the
	// given entities in the local model.
	ExportEntities([]names.Tag) ([]params.RemoteEntityIdResult, error)

	// GetToken returns the token associated with the entity with the given tag
	// for the specified model.
	GetToken(string, names.Tag) (string, error)

	// RemoveRemoteEntity removes the specified entity from the remote entities collection.
	RemoveRemoteEntity(sourceModelUUID string, entity names.Tag) error

	// RelationUnitSettings returns the relation unit settings for the
	// given relation units in the local model.
	RelationUnitSettings([]params.RelationUnit) ([]params.SettingsResult, error)

	// Relations returns information about the relations
	// with the specified keys in the local model.
	Relations(keys []string) ([]params.RemoteRelationResult, error)

	// RemoteApplications returns the current state of the remote applications with
	// the specified names in the local model.
	RemoteApplications(names []string) ([]params.RemoteApplicationResult, error)

	// WatchLocalRelationUnits returns a watcher that notifies of changes to the
	// local units in the relation with the given key.
	WatchLocalRelationUnits(relationKey string) (watcher.RelationUnitsWatcher, error)

	// WatchRemoteApplications watches for addition, removal and lifecycle
	// changes to remote applications known to the local model.
	WatchRemoteApplications() (watcher.StringsWatcher, error)

	// WatchRemoteApplicationRelations starts a StringsWatcher for watching the relations of
	// each specified application in the local environment, and returns the watcher IDs
	// and initial values, or an error if the applications' relations could not be
	// watched.
	WatchRemoteApplicationRelations(application string) (watcher.StringsWatcher, error)

	// ConsumeRemoteRelationChange consumes a change to settings originating
	// from the remote/offering side of a relation.
	ConsumeRemoteRelationChange(change params.RemoteRelationChangeEvent) error
}

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID                string
	RelationsFacade          RemoteRelationsFacade
	NewRemoteModelFacadeFunc func(modelUUID string) (RemoteModelRelationsFacadeCloser, error)
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.ModelUUID == "" {
		return errors.NotValidf("empty model uuid")
	}
	if config.RelationsFacade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.NewRemoteModelFacadeFunc == nil {
		return errors.NotValidf("nil Remote Model Facade func")
	}
	return nil
}

// New returns a Worker backed by config, or an error.
func New(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config:             config,
		logger:             logger,
		applicationWorkers: make(map[string]worker.Worker),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

// Worker manages relations and associated settings where
// one end of the relation is a remote application.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	logger   loggo.Logger

	// applicationWorkers holds a worker for each
	// remote application being watched.
	applicationWorkers map[string]worker.Worker
}

// Kill is defined on worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() (err error) {
	changes, err := w.config.RelationsFacade.WatchRemoteApplications()
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
			err = w.handleApplicationChanges(applicationIds)
			if err != nil {
				return err
			}
		}
	}
}

func (w *Worker) handleApplicationChanges(applicationIds []string) error {
	// TODO(wallyworld) - watcher should not give empty events
	if len(applicationIds) == 0 {
		return nil
	}
	logger.Debugf("processing remote application changes for: %s", applicationIds)

	// Fetch the current state of each of the remote applications that have changed.
	results, err := w.config.RelationsFacade.RemoteApplications(applicationIds)
	if err != nil {
		return errors.Annotate(err, "querying remote applications")
	}

	for i, result := range results {
		name := applicationIds[i]
		if result.Error != nil {
			// The the remote application has been removed, stop its worker.
			if params.IsCodeNotFound(result.Error) {
				if err := w.killApplicationWorker(name); err != nil {
					return err
				}
				continue
			}
			return errors.Annotatef(err, "querying remote application %q", name)
		}
		if _, ok := w.applicationWorkers[name]; ok {
			// TODO(wallyworld): handle application dying or dead.
			// As of now, if the worker is already running, that's all we need.
			continue
		}
		relationsWatcher, err := w.config.RelationsFacade.WatchRemoteApplicationRelations(name)
		if errors.IsNotFound(err) {
			if err := w.killApplicationWorker(name); err != nil {
				return err
			}
			continue
		} else if err != nil {
			return errors.Annotatef(err, "watching relations for remote application %q", name)
		}
		logger.Debugf("started watcher for remote application %q", name)
		appWorker, err := newRemoteApplicationWorker(
			relationsWatcher,
			w.config.ModelUUID,
			*result.Result,
			w.config.NewRemoteModelFacadeFunc,
			w.config.RelationsFacade,
		)
		if err != nil {
			return errors.Trace(err)
		}
		if err := w.catacomb.Add(appWorker); err != nil {
			return errors.Trace(err)
		}
		w.applicationWorkers[name] = appWorker
	}
	return nil
}

func (w *Worker) killApplicationWorker(name string) error {
	appWorker, ok := w.applicationWorkers[name]
	if ok {
		delete(w.applicationWorkers, name)
		return worker.Stop(appWorker)
	}
	return nil
}

// remoteApplicationWorker listens for localChanges to relations
// involving a remote application, and publishes change to
// local relation units to the remote model. It also watches for
// changes originating from the offering model and consumes those
// in the local model.
type remoteApplicationWorker struct {
	catacomb              catacomb.Catacomb
	relationsWatcher      watcher.StringsWatcher
	relationInfo          remoteRelationInfo
	localModelUUID        string // uuid of the model hosting the local application
	remoteModelUUID       string // uuid of the model hosting the remote application
	registered            bool
	localRelationChanges  chan params.RemoteRelationChangeEvent
	remoteRelationChanges chan params.RemoteRelationChangeEvent

	// macaroon is used to confirm that permission has been granted to consume
	// the remote application to which this worker pertains.
	macaroon *macaroon.Macaroon

	// localModelFacade interacts with the local (consuming) model.
	localModelFacade RemoteRelationsFacade
	// remoteModelFacade interacts with the remote (offering) model.
	remoteModelFacade RemoteModelRelationsFacadeCloser

	newRemoteModelRelationsFacadeFunc func(modelUUID string) (RemoteModelRelationsFacadeCloser, error)
}

type relation struct {
	relationId int
	life       params.Life
	localRuw   *relationUnitsWorker
	remoteRuw  *relationUnitsWorker
}

type remoteRelationInfo struct {
	localApplicationId         params.RemoteEntityId
	localEndpoint              params.RemoteEndpoint
	remoteApplicationName      string
	remoteApplicationOfferName string
	remoteEndpointName         string
}

func newRemoteApplicationWorker(
	relationsWatcher watcher.StringsWatcher,
	localModelUUID string,
	remoteApplication params.RemoteApplication,
	newRemoteModelRelationsFacadeFunc func(modelUUID string) (RemoteModelRelationsFacadeCloser, error),
	facade RemoteRelationsFacade,
) (worker.Worker, error) {
	w := &remoteApplicationWorker{
		relationsWatcher: relationsWatcher,
		relationInfo: remoteRelationInfo{
			remoteApplicationOfferName: remoteApplication.OfferName,
			remoteApplicationName:      remoteApplication.Name,
		},
		localModelUUID:                    localModelUUID,
		remoteModelUUID:                   remoteApplication.ModelUUID,
		registered:                        remoteApplication.Registered,
		macaroon:                          remoteApplication.Macaroon,
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
	var err error
	w.remoteModelFacade, err = w.newRemoteModelRelationsFacadeFunc(w.remoteModelUUID)
	if err != nil {
		return errors.Annotate(err, "opening facade to remote model")
	}
	defer w.remoteModelFacade.Close()

	relations := make(map[string]*relation)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-w.relationsWatcher.Changes():
			logger.Debugf("relations changed: %#v, %v", change, ok)
			if !ok {
				// We are dying.
				continue
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

	// Remove the remote entity record for the relation to ensure any unregister
	// call from the remote model that may come across at the same time is short circuited.
	remoteId := relation.localRuw.remoteRelationId
	relTag := names.NewRelationTag(key)
	_, err := w.localModelFacade.GetToken(remoteId.ModelUUID, relTag)
	if errors.IsNotFound(err) {
		logger.Debugf("not found token for %v in %v, exit early", key, w.localModelUUID)
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// We also need to remove the remote entity reference for the relation.
	if err := w.localModelFacade.RemoveRemoteEntity(remoteId.ModelUUID, relTag); err != nil {
		return errors.Trace(err)
	}

	// On the consuming side, inform the remote side the relation has died.
	if !w.registered {
		change := params.RemoteRelationChangeEvent{
			RelationId:    remoteId,
			Life:          params.Dead,
			ApplicationId: w.relationInfo.localApplicationId,
			Macaroon:      w.macaroon,
		}
		if err := w.remoteModelFacade.PublishRelationChange(change); err != nil {
			return errors.Annotatef(err, "publishing relation departed %+v to remote model %v", change, w.remoteModelUUID)
		}
	}
	// TODO(wallyworld) - on the offering side, ensure the consuming watcher learns about the removal
	logger.Debugf("remote relation %v removed from remote model", key)

	// TODO(wallyworld) - check that state cleanup worker properly removes the dead relation.
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
	// relation is now dead, stop the watcher.
	if r := relations[key]; r != nil {
		r.life = remoteRelation.Life
		if r.life == params.Dead {
			return w.processRelationGone(key, relations)
		}
		// Nothing to do, we have previously started the watcher.
		return nil
	}
	if remoteRelation.Life == params.Dead {
		// We haven't started the relation unit watcher so just exit.
		return nil
	}
	if w.registered {
		return w.processNewOfferingRelation(remoteRelation.ApplicationName, key)
	}
	return w.processNewConsumingRelation(key, relations, remoteRelation)
}

func (w *remoteApplicationWorker) processNewOfferingRelation(applicationName string, key string) error {
	// We are on the offering side and the relation has been registered,
	// so look up the token to use when communicating status.
	relationTag := names.NewRelationTag(key)
	token, err := w.localModelFacade.GetToken(w.remoteModelUUID, relationTag)
	if err != nil {
		return errors.Annotatef(err, "getting token for relation %v from consuming model", relationTag.Id())
	}
	// Look up the exported token of the local application in the relation.
	// The export was done when the relation was registered.
	token, err = w.localModelFacade.GetToken(w.localModelUUID, names.NewApplicationTag(applicationName))
	if err != nil {
		return errors.Annotatef(err, "getting token for application %v from offering model", applicationName)
	}
	w.relationInfo.localApplicationId = params.RemoteEntityId{ModelUUID: w.localModelUUID, Token: token}
	return nil
}

// processNewConsumingRelation starts the sub-workers necessary to listen and publish
// local unit settings changes, and watch and consume remote unit settings changes.
func (w *remoteApplicationWorker) processNewConsumingRelation(
	key string,
	relations map[string]*relation,
	remoteRelation *params.RemoteRelation,
) error {
	// We have not seen the relation before, make
	// sure it is registered on the offering side.
	w.relationInfo.localEndpoint = remoteRelation.Endpoint
	w.relationInfo.remoteEndpointName = remoteRelation.RemoteEndpointName

	applicationTag := names.NewApplicationTag(remoteRelation.ApplicationName)
	relationTag := names.NewRelationTag(key)
	applicationId, remoteApplictionId, relationId, err := w.registerRemoteRelation(applicationTag, relationTag)
	if err != nil {
		return errors.Annotatef(err, "registering application %v and relation %v", remoteRelation.ApplicationName, relationTag.Id())
	}
	w.relationInfo.localApplicationId = applicationId

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
		applicationId,
		w.macaroon,
		relationId,
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
	remoteRelationUnitsWatcher, err := w.remoteModelFacade.WatchRelationUnits(relationId)
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
				RelationId: relationId,
				Unit:       names.NewUnitTag(changedName).String(),
			}
		}
		return w.remoteModelFacade.RelationUnitSettings(relationUnits)
	}
	remoteUnitsWorker, err := newRelationUnitsWorker(
		relationTag,
		remoteApplictionId,
		w.macaroon,
		relationId,
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

	relations[key] = &relation{
		relationId: remoteRelation.Id,
		life:       remoteRelation.Life,
		localRuw:   localUnitsWorker,
		remoteRuw:  remoteUnitsWorker,
	}

	return nil
}

func (w *remoteApplicationWorker) registerRemoteRelation(
	applicationTag, relationTag names.Tag,
) (localApplicationId, offeringRemoteAppId, relationId params.RemoteEntityId, _ error) {
	logger.Debugf("register remote relation %v", relationTag.Id())

	emptyId := params.RemoteEntityId{}
	fail := func(err error) (params.RemoteEntityId, params.RemoteEntityId, params.RemoteEntityId, error) {
		return emptyId, emptyId, emptyId, err
	}

	// Ensure the relation is exported first up.
	results, err := w.localModelFacade.ExportEntities([]names.Tag{applicationTag, relationTag})
	if err != nil {
		return fail(errors.Annotatef(err, "exporting relation %v and application", relationTag, applicationTag))
	}
	if results[0].Error != nil && !params.IsCodeAlreadyExists(results[0].Error) {
		return fail(errors.Annotatef(err, "exporting application %v", applicationTag))
	}
	localApplicationId = *results[0].Result
	if results[1].Error != nil && !params.IsCodeAlreadyExists(results[1].Error) {
		return fail(errors.Annotatef(err, "exporting relation %v", relationTag))
	}
	relationId = *results[1].Result

	// This data goes to the remote model so we map local info
	// from this model to the remote arg values and visa versa.
	arg := params.RegisterRemoteRelation{
		ApplicationId:     localApplicationId,
		RelationId:        relationId,
		RemoteEndpoint:    w.relationInfo.localEndpoint,
		OfferName:         w.relationInfo.remoteApplicationOfferName,
		LocalEndpointName: w.relationInfo.remoteEndpointName,
		Macaroon:          w.macaroon,
	}
	remoteAppIds, err := w.remoteModelFacade.RegisterRemoteRelations(arg)
	if err != nil {
		return fail(errors.Trace(err))
	}
	// remoteAppIds is a slice but there's only one item
	// as we currently only register one remote application
	if err := remoteAppIds[0].Error; err != nil {
		return fail(errors.Trace(err))
	}
	if err := results[0].Error; err != nil && !params.IsCodeAlreadyExists(err) {
		return fail(errors.Annotatef(err, "registering relation %v", relationTag))
	}
	// Import the application id from the offering model.
	offeringRemoteAppId = *remoteAppIds[0].Result
	logger.Debugf("import remote application token %v from %v for %v",
		offeringRemoteAppId.Token, offeringRemoteAppId.ModelUUID, w.relationInfo.remoteApplicationName)
	err = w.localModelFacade.ImportRemoteEntity(
		offeringRemoteAppId.ModelUUID,
		names.NewApplicationTag(w.relationInfo.remoteApplicationName),
		offeringRemoteAppId.Token)
	if err != nil && !params.IsCodeAlreadyExists(err) {
		return fail(errors.Annotatef(
			err, "importing remote application %v to local model", w.relationInfo.remoteApplicationName))
	}
	return localApplicationId, offeringRemoteAppId, relationId, nil
}

type relationUnitsSettingsFunc func([]string) ([]params.SettingsResult, error)

// relationUnitsWorker uses instances of watcher.RelationUnitsWatcher to
// listen to changes to relation settings in a model, local or remote.
// Local changes are exported to the remote model.
// Remote changes are consumed by the local model.
type relationUnitsWorker struct {
	catacomb    catacomb.Catacomb
	relationTag names.RelationTag
	ruw         watcher.RelationUnitsWatcher
	changes     chan<- params.RemoteRelationChangeEvent

	applicationId    params.RemoteEntityId
	macaroon         *macaroon.Macaroon
	remoteRelationId params.RemoteEntityId
	remoteUnitIds    map[string]params.RemoteEntityId

	unitSettingsFunc relationUnitsSettingsFunc
}

func newRelationUnitsWorker(
	relationTag names.RelationTag,
	applicationId params.RemoteEntityId,
	macaroon *macaroon.Macaroon,
	remoteRelationId params.RemoteEntityId,
	ruw watcher.RelationUnitsWatcher,
	changes chan<- params.RemoteRelationChangeEvent,
	unitSettingsFunc relationUnitsSettingsFunc,
) (*relationUnitsWorker, error) {
	w := &relationUnitsWorker{
		relationTag:      relationTag,
		applicationId:    applicationId,
		macaroon:         macaroon,
		remoteRelationId: remoteRelationId,
		ruw:              ruw,
		changes:          changes,
		remoteUnitIds:    make(map[string]params.RemoteEntityId),
		unitSettingsFunc: unitSettingsFunc,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{ruw},
	})
	return w, err
}

// Kill is defined on worker.Worker
func (w *relationUnitsWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *relationUnitsWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *relationUnitsWorker) loop() error {
	var (
		changes chan<- params.RemoteRelationChangeEvent
		event   params.RemoteRelationChangeEvent
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-w.ruw.Changes():
			if !ok {
				// We are dying.
				continue
			}
			logger.Debugf("relation units changed for %v: %#v", w.relationTag, change)
			if evt, err := w.relationUnitsChangeEvent(change); err != nil {
				return errors.Trace(err)
			} else {
				if evt == nil {
					continue
				}
				event = *evt
				changes = w.changes
			}
		case changes <- event:
			changes = nil
		}
	}
}

func (w *relationUnitsWorker) relationUnitsChangeEvent(
	change watcher.RelationUnitsChange,
) (*params.RemoteRelationChangeEvent, error) {
	logger.Debugf("update relation units for %v", w.relationTag)
	if len(change.Changed)+len(change.Departed) == 0 {
		return nil, nil
	}
	// Ensure all the changed units have been exported.
	changedUnitNames := make([]string, 0, len(change.Changed))
	for name := range change.Changed {
		changedUnitNames = append(changedUnitNames, name)
	}

	// unitNum parses a unit name and extracts the unit number.
	unitNum := func(unitName string) (int, error) {
		parts := strings.Split(unitName, "/")
		if len(parts) < 2 {
			return -1, errors.NotValidf("unit name %v", unitName)
		}
		return strconv.Atoi(parts[1])
	}

	// Construct the event to send to the remote model.
	event := &params.RemoteRelationChangeEvent{
		RelationId:    w.remoteRelationId,
		Life:          params.Alive,
		ApplicationId: w.applicationId,
		Macaroon:      w.macaroon,
		DepartedUnits: make([]int, len(change.Departed)),
	}
	for i, u := range change.Departed {
		num, err := unitNum(u)
		if err != nil {
			return nil, errors.Trace(err)
		}
		event.DepartedUnits[i] = num
	}

	if len(changedUnitNames) > 0 {
		// For changed units, we publish/consume the current settings values.
		results, err := w.unitSettingsFunc(changedUnitNames)
		if err != nil {
			return nil, errors.Annotate(err, "fetching relation units settings")
		}
		for i, result := range results {
			if result.Error != nil {
				return nil, errors.Annotatef(result.Error, "fetching relation unit settings for %v", changedUnitNames[i])
			}
		}
		for i, result := range results {
			num, err := unitNum(changedUnitNames[i])
			if err != nil {
				return nil, errors.Trace(err)
			}
			change := params.RemoteRelationUnitChange{
				UnitId:   num,
				Settings: make(map[string]interface{}),
			}
			for k, v := range result.Settings {
				change.Settings[k] = v
			}
			event.ChangedUnits = append(event.ChangedUnits, change)
		}
	}
	return event, nil
}
