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

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.remoterelations")

// RemoteRelationChangePublisherCloser implements RemoteRelationChangePublisher
// and add a Close() method.
type RemoteRelationChangePublisherCloser interface {
	io.Closer
	RemoteRelationChangePublisher
}

// RemoteRelationChangePublisher instances publish local relation changes to the
// model hosting the remote application involved in the relation
type RemoteRelationChangePublisher interface {
	// RegisterRemoteRelation sets up the local model to participate
	// in the specified relations.
	RegisterRemoteRelations(relations ...params.RegisterRemoteRelation) ([]params.RemoteEntityIdResult, error)

	// PublishLocalRelationChange publishes local relation changes to the
	// model hosting the remote application involved in the relation.
	PublishLocalRelationChange(params.RemoteRelationChangeEvent) error
}

// RemoteRelationsFacade exposes remote relation functionality to a worker.
type RemoteRelationsFacade interface {
	RemoteRelationChangePublisher

	// ImportRemoteEntity adds an entity to the remote entities collection
	// with the specified opaque token.
	ImportRemoteEntity(sourceModelUUID string, entity names.Tag, token string) error

	// ExportEntities allocates unique, remote entity IDs for the
	// given entities in the local model.
	ExportEntities([]names.Tag) ([]params.RemoteEntityIdResult, error)

	// GetToken returns the token associated with the entity with the given tag
	// for the specified model.
	GetToken(string, names.Tag) (string, error)

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
}

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID                string
	RelationsFacade          RemoteRelationsFacade
	NewPublisherForModelFunc func(modelUUID string) (RemoteRelationChangePublisherCloser, error)
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.ModelUUID == "" {
		return errors.NotValidf("empty model uuid")
	}
	if config.RelationsFacade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.NewPublisherForModelFunc == nil {
		return errors.NotValidf("nil Publisher func")
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
			w.config.NewPublisherForModelFunc,
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

// remoteApplicationWorker listens for changes to relations
// involving a remote application, and publishes changes to
// local relation units to the remote model.
type remoteApplicationWorker struct {
	catacomb         catacomb.Catacomb
	relationsWatcher watcher.StringsWatcher
	relationInfo     remoteRelationInfo
	localModelUUID   string // uuid of the model hosting the local application
	remoteModelUUID  string // uuid of the model hosting the remote application
	registered       bool
	relationChanges  chan params.RemoteRelationChangeEvent

	facade                   RemoteRelationsFacade
	newPublisherForModelFunc func(modelUUID string) (RemoteRelationChangePublisherCloser, error)
}

type relation struct {
	params.RemoteRelationChange
	ruw *relationUnitsWatcher
}

type remoteRelationInfo struct {
	applicationId          params.RemoteEntityId
	localEndpoint          params.RemoteEndpoint
	remoteApplicationAlias string
	remoteApplicationName  string
	remoteEndpointName     string
}

func newRemoteApplicationWorker(
	relationsWatcher watcher.StringsWatcher,
	localModelUUID string,
	remoteApplication params.RemoteApplication,
	newPublisherForModelFunc func(modelUUID string) (RemoteRelationChangePublisherCloser, error),
	facade RemoteRelationsFacade,
) (worker.Worker, error) {
	// We store the remote application name locally as an alias <user>-<model>-<appname>.
	// For now, the name of the offering side can be deduced from the alias.
	// TODO(wallyworld) - record the offering name in the RemoteApplication doc to allow arbitrary aliases
	offeringName := remoteApplication.Name
	appNameParts := strings.Split(remoteApplication.Name, "-")
	if len(appNameParts) == 3 {
		offeringName = appNameParts[2]
	}
	w := &remoteApplicationWorker{
		relationsWatcher: relationsWatcher,
		relationInfo: remoteRelationInfo{
			remoteApplicationName:  offeringName,
			remoteApplicationAlias: remoteApplication.Name,
		},
		localModelUUID:  localModelUUID,
		remoteModelUUID: remoteApplication.ModelUUID,
		registered:      remoteApplication.Registered,
		relationChanges: make(chan params.RemoteRelationChangeEvent),
		facade:          facade,
		newPublisherForModelFunc: newPublisherForModelFunc,
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
	publisher, err := w.newPublisherForModelFunc(w.remoteModelUUID)
	if err != nil {
		return errors.Annotate(err, "opening publisher to remote model")
	}
	defer publisher.Close()

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
			results, err := w.facade.Relations(change)
			if err != nil {
				return errors.Annotate(err, "querying relations")
			}
			for i, result := range results {
				key := change[i]
				if err := w.relationChanged(key, result, relations, publisher); err != nil {
					return errors.Annotatef(err, "handling change for relation %q", key)
				}
			}
		case change := <-w.relationChanges:
			logger.Debugf("relation units changed: %#v", change)
			if err := publisher.PublishLocalRelationChange(change); err != nil {
				return errors.Annotatef(err, "publishing relation change %+v to remote model %v", change, w.remoteModelUUID)
			}
		}
	}
}

func (w *remoteApplicationWorker) killRelationUnitWatcher(key string, relations map[string]*relation) error {
	relation, ok := relations[key]
	if ok {
		delete(relations, key)
		return worker.Stop(relation.ruw)
	}
	return nil
}

func (w *remoteApplicationWorker) relationChanged(
	key string, result params.RemoteRelationResult, relations map[string]*relation,
	publisher RemoteRelationChangePublisher,
) error {
	logger.Debugf("relation %q changed: %+v", key, result)
	if result.Error != nil {
		if params.IsCodeNotFound(result.Error) {
			// TODO(wallyworld) - once a relation dies, wait for
			// it to be unregistered from remote side and then use
			// cleanup to remove.
			return w.killRelationUnitWatcher(key, relations)
		}
		return result.Error
	}
	remoteRelation := result.Result

	// If we have previously started the watcher and the
	// relation is now dead, stop the watcher.
	relationTag := names.NewRelationTag(key)
	if r := relations[key]; r != nil {
		r.Life = remoteRelation.Life
		if r.Life == params.Dead {
			return w.killRelationUnitWatcher(key, relations)
		}
		// Nothing to do, we have previously started the watcher.
		return nil
	}
	if remoteRelation.Life == params.Dead {
		// We haven't started the relation unit watcher so just exit.
		return nil
	}

	var remoteRelationId params.RemoteEntityId
	if w.registered {
		// We are on the offering side and the relation has been registered,
		// so look up the token to use when communicating status.
		token, err := w.facade.GetToken(w.remoteModelUUID, relationTag)
		if err != nil {
			return errors.Trace(err)
		}
		remoteRelationId = params.RemoteEntityId{ModelUUID: w.remoteModelUUID, Token: token}
		// Look up the exported token of the local application in the relation.
		// The export was done when the relation was registered.
		token, err = w.facade.GetToken(w.localModelUUID, names.NewApplicationTag(remoteRelation.ApplicationName))
		if err != nil {
			return errors.Trace(err)
		}
		w.relationInfo.applicationId = params.RemoteEntityId{ModelUUID: w.localModelUUID, Token: token}
	} else {
		// We have not seen the relation before, make
		// sure it is registered on the offering side.
		w.relationInfo.localEndpoint = remoteRelation.Endpoint
		w.relationInfo.remoteEndpointName = remoteRelation.RemoteEndpointName
		var err error
		applicationTag := names.NewApplicationTag(remoteRelation.ApplicationName)
		w.relationInfo.applicationId, remoteRelationId, err = w.registerRemoteRelation(applicationTag, relationTag, publisher)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Start a watcher to track changes to the local units in the
	// relation, and a worker to process those changes.
	localRelationUnitsWatcher, err := w.facade.WatchLocalRelationUnits(key)
	if err != nil {
		return errors.Trace(err)
	}
	relationUnitsWatcher, err := newRelationUnitsWatcher(
		relationTag,
		w.relationInfo.applicationId,
		remoteRelationId,
		localRelationUnitsWatcher,
		w.relationChanges,
		w.facade,
	)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(relationUnitsWatcher); err != nil {
		return errors.Trace(err)
	}
	relations[key] = &relation{
		RemoteRelationChange: params.RemoteRelationChange{
			RelationId: remoteRelation.Id,
			Life:       remoteRelation.Life,
		},
		ruw: relationUnitsWatcher,
	}
	return nil
}

func (w *remoteApplicationWorker) registerRemoteRelation(
	applicationTag, relationTag names.Tag, publisher RemoteRelationChangePublisher,
) (remoteApplicationId params.RemoteEntityId, remoteRelationId params.RemoteEntityId, _ error) {
	logger.Debugf("register remote relation %v", relationTag.Id())
	emptyId := params.RemoteEntityId{}
	// Ensure the relation is exported first up.
	results, err := w.facade.ExportEntities([]names.Tag{applicationTag, relationTag})
	if err != nil {
		return emptyId, emptyId, errors.Annotatef(err, "exporting relation %v and application", relationTag, applicationTag)
	}
	if results[0].Error != nil && !params.IsCodeAlreadyExists(results[0].Error) {
		return emptyId, emptyId, errors.Annotatef(err, "exporting application %v", applicationTag)
	}
	remoteApplicationId = *results[0].Result
	if results[1].Error != nil && !params.IsCodeAlreadyExists(results[1].Error) {
		return emptyId, emptyId, errors.Annotatef(err, "exporting relation %v", relationTag)
	}
	remoteRelationId = *results[1].Result

	// This data goes to the remote model so we map local info
	// from this model to the remote arg values and visa versa.
	arg := params.RegisterRemoteRelation{
		ApplicationId:          remoteApplicationId,
		RelationId:             remoteRelationId,
		RemoteEndpoint:         w.relationInfo.localEndpoint,
		OfferedApplicationName: w.relationInfo.remoteApplicationName,
		LocalEndpointName:      w.relationInfo.remoteEndpointName,
	}
	remoteAppIds, err := publisher.RegisterRemoteRelations(arg)
	if err != nil {
		return emptyId, emptyId, errors.Trace(err)
	}
	// remoteAppIds is a slice but there's only one item
	// as we currently only register one remote application
	if err := remoteAppIds[0].Error; err != nil {
		return emptyId, emptyId, errors.Trace(err)
	}
	if err := results[0].Error; err != nil && !params.IsCodeAlreadyExists(err) {
		return emptyId, emptyId, errors.Annotatef(err, "registering relation %v", relationTag)
	}
	// Import the application id from the offering model.
	offeringRemoteAppId := remoteAppIds[0].Result
	logger.Debugf("import remote application token %v from %v for %v",
		offeringRemoteAppId.Token, offeringRemoteAppId.ModelUUID, w.relationInfo.remoteApplicationAlias)
	err = w.facade.ImportRemoteEntity(
		offeringRemoteAppId.ModelUUID,
		names.NewApplicationTag(w.relationInfo.remoteApplicationAlias),
		offeringRemoteAppId.Token)
	if err != nil && !params.IsCodeAlreadyExists(err) {
		return emptyId, emptyId, errors.Annotatef(
			err, "importing remote application %v to local model", w.relationInfo.remoteApplicationAlias)
	}
	return remoteApplicationId, remoteRelationId, nil
}

// relationUnitsWatcher uses a watcher.RelationUnitsWatcher to listen
// to changes to relation settings in the local model and converts
// to a params.RemoteRelationChanges for export to a remote model.
type relationUnitsWatcher struct {
	catacomb    catacomb.Catacomb
	relationTag names.RelationTag
	ruw         watcher.RelationUnitsWatcher
	changes     chan<- params.RemoteRelationChangeEvent

	applicationId    params.RemoteEntityId
	remoteRelationId params.RemoteEntityId
	remoteUnitIds    map[string]params.RemoteEntityId

	facade RemoteRelationsFacade
}

func newRelationUnitsWatcher(
	relationTag names.RelationTag,
	applicationId params.RemoteEntityId,
	remoteRelationId params.RemoteEntityId,
	ruw watcher.RelationUnitsWatcher,
	changes chan<- params.RemoteRelationChangeEvent,

	facade RemoteRelationsFacade,
) (*relationUnitsWatcher, error) {
	w := &relationUnitsWatcher{
		relationTag:      relationTag,
		applicationId:    applicationId,
		remoteRelationId: remoteRelationId,
		ruw:              ruw,
		changes:          changes,
		remoteUnitIds:    make(map[string]params.RemoteEntityId),
		facade:           facade,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{ruw},
	})
	return w, err
}

// Kill is defined on worker.Worker
func (w *relationUnitsWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *relationUnitsWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *relationUnitsWatcher) loop() error {
	var changes chan<- params.RemoteRelationChangeEvent
	var event params.RemoteRelationChangeEvent
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

func (w *relationUnitsWatcher) relationUnitsChangeEvent(
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
		DepartedUnits: make([]int, len(change.Departed)),
	}
	for i, u := range change.Departed {
		num, err := unitNum(u)
		if err != nil {
			return nil, errors.Trace(err)
		}
		event.DepartedUnits[i] = num
	}

	if len(change.Changed) > 0 {
		// For changed units, we publish the current settings values.
		relationUnits := make([]params.RelationUnit, len(change.Changed))
		for i, changedName := range changedUnitNames {
			relationUnits[i] = params.RelationUnit{
				Relation: w.relationTag.String(),
				Unit:     names.NewUnitTag(changedName).String(),
			}
		}
		results, err := w.facade.RelationUnitSettings(relationUnits)
		if err != nil {
			return nil, errors.Annotate(err, "fetching relation units settings")
		}
		for i, result := range results {
			if result.Error != nil {
				return nil, errors.Annotatef(result.Error, "fetching relation unit settings for %v", relationUnits[i].Unit)
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
