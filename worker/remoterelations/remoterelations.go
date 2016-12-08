// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"io"
	"sync"

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
	// in the specified relation.
	RegisterRemoteRelation(rel params.RegisterRemoteRelation) error

	// PublishLocalRelationChange publishes local relation changes to the
	// model hosting the remote application involved in the relation.
	PublishLocalRelationChange(params.RemoteRelationChangeEvent) error
}

// RemoteRelationsFacade exposes remote relation functionality to a worker.
type RemoteRelationsFacade interface {
	RemoteRelationChangePublisher

	// ExportEntities allocates unique, remote entity IDs for the
	// given entities in the local model.
	ExportEntities([]names.Tag) ([]params.RemoteEntityIdResult, error)

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
	RelationsFacade          RemoteRelationsFacade
	NewPublisherForModelFunc func(modelUUID string) (RemoteRelationChangePublisherCloser, error)
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.RelationsFacade == nil {
		return errors.NotValidf("nil Facade")
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

	// exportMutex is used to ensure only one export API call from
	// the relation unit watcher can occur at any time.
	// This prevents the possibility of the same unit being exported
	// simultaneously.
	exportMutex sync.Mutex

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
		// A new remote application has appeared, start monitoring relations to it
		// originating from the local model.
		// First, export the application.
		appTag := names.NewApplicationTag(name)
		results, err := w.config.RelationsFacade.ExportEntities([]names.Tag{appTag})
		if err != nil {
			return errors.Annotatef(err, "exporting application %v", appTag)
		}
		if results[0].Error != nil && !params.IsCodeAlreadyExists(results[0].Error) {
			return errors.Annotatef(err, "exporting application %v", appTag)
		}
		// Record what we know so far - the other attributes of
		// the relation info will be filled in later.
		relationInfo := remoteRelationInfo{
			applicationId:         *results[0].Result,
			remoteApplicationName: name,
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
			relationInfo,
			result.Result.ModelUUID,
			result.Result.Registered,
			w.config.NewPublisherForModelFunc,
			w.config.RelationsFacade,
			&w.exportMutex,
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
	modelUUID        string // uuid of the model hosting the remote application
	registered       bool
	relationChanges  chan params.RemoteRelationChangeEvent

	exportMutex              *sync.Mutex
	facade                   RemoteRelationsFacade
	newPublisherForModelFunc func(modelUUID string) (RemoteRelationChangePublisherCloser, error)
}

type relation struct {
	params.RemoteRelationChange
	ruw *relationUnitsWatcher
}

type remoteRelationInfo struct {
	applicationId         params.RemoteEntityId
	localEndpoint         params.RemoteEndpoint
	remoteApplicationName string
	remoteEndpointName    string
}

func newRemoteApplicationWorker(
	relationsWatcher watcher.StringsWatcher,
	relationInfo remoteRelationInfo,
	modelUUID string,
	registered bool,
	newPublisherForModelFunc func(modelUUID string) (RemoteRelationChangePublisherCloser, error),
	facade RemoteRelationsFacade,
	exportMutex *sync.Mutex,
) (worker.Worker, error) {
	w := &remoteApplicationWorker{
		relationsWatcher:         relationsWatcher,
		relationInfo:             relationInfo,
		modelUUID:                modelUUID,
		registered:               registered,
		relationChanges:          make(chan params.RemoteRelationChangeEvent),
		facade:                   facade,
		exportMutex:              exportMutex,
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
	publisher, err := w.newPublisherForModelFunc(w.modelUUID)
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
				return errors.Annotate(err, "publishing relation change to remote model")
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
	var remoteRelationId params.RemoteEntityId
	relationTag := names.NewRelationTag(key)
	if r := relations[key]; r != nil {
		r.Life = remoteRelation.Life
		if r.Life == params.Dead {
			return w.killRelationUnitWatcher(key, relations)
		}
		// Nothing to do, we have previously started the watcher.
		return nil
	} else if !w.registered {
		// We have not seen the relation before, make
		// sure it is registered on the remote side.
		w.relationInfo.localEndpoint = remoteRelation.LocalEndpoint
		w.relationInfo.remoteEndpointName = remoteRelation.RemoteEndpointName
		var err error
		remoteRelationId, err = w.registerRemoteRelation(relationTag, publisher)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Start a watcher to track changes to the local units in the
	// relation, and a worker to process those changes.
	if remoteRelation.Life != params.Dead {
		localRelationUnitsWatcher, err := w.facade.WatchLocalRelationUnits(key)
		if err != nil {
			return errors.Trace(err)
		}
		relationUnitsWatcher, err := newRelationUnitsWatcher(
			relationTag,
			remoteRelationId,
			localRelationUnitsWatcher,
			w.relationChanges,
			w.facade,
			w.exportMutex,
		)
		if err != nil {
			return errors.Trace(err)
		}
		if err := w.catacomb.Add(relationUnitsWatcher); err != nil {
			return errors.Trace(err)
		}
		r := &relation{}
		r.RelationId = remoteRelation.Id
		r.Life = remoteRelation.Life
		r.ruw = relationUnitsWatcher
		relations[key] = r
	}
	return nil
}

func (w *remoteApplicationWorker) registerRemoteRelation(
	relationTag names.Tag, publisher RemoteRelationChangePublisher,
) (params.RemoteEntityId, error) {
	// Ensure the relation is exported first up.
	results, err := w.facade.ExportEntities([]names.Tag{relationTag})
	if err != nil {
		return params.RemoteEntityId{}, errors.Annotatef(err, "exporting relation %v", relationTag)
	}
	if results[0].Error != nil && !params.IsCodeAlreadyExists(results[0].Error) {
		return params.RemoteEntityId{}, errors.Annotatef(err, "exporting relation %v", relationTag)
	}
	remoteRelationId := *results[0].Result

	// This data goes to the remote model so we map local info
	// from this model to the remote arg vales and visa versa.
	arg := params.RegisterRemoteRelation{
		ApplicationId:          w.relationInfo.applicationId,
		RelationId:             remoteRelationId,
		RemoteEndpoint:         w.relationInfo.localEndpoint,
		OfferedApplicationName: w.relationInfo.remoteApplicationName,
		LocalEndpointName:      w.relationInfo.remoteEndpointName,
	}
	if err := publisher.RegisterRemoteRelation(arg); err != nil {
		return params.RemoteEntityId{}, errors.Trace(err)
	}
	return remoteRelationId, nil
}

// relationUnitsWatcher uses a watcher.RelationUnitsWatcher to listen
// to changes to relation settings in the local model and converts
// to a params.RemoteRelationChanges for export to a remote model.
type relationUnitsWatcher struct {
	catacomb    catacomb.Catacomb
	relationTag names.RelationTag
	ruw         watcher.RelationUnitsWatcher
	changes     chan<- params.RemoteRelationChangeEvent

	remoteRelationId params.RemoteEntityId
	remoteUnitIds    map[string]params.RemoteEntityId

	exportMutex *sync.Mutex
	facade      RemoteRelationsFacade
}

func newRelationUnitsWatcher(
	relationTag names.RelationTag,
	remoteRelationId params.RemoteEntityId,
	ruw watcher.RelationUnitsWatcher,
	changes chan<- params.RemoteRelationChangeEvent,

	facade RemoteRelationsFacade,
	exportMutex *sync.Mutex,
) (*relationUnitsWatcher, error) {
	w := &relationUnitsWatcher{
		relationTag:      relationTag,
		remoteRelationId: remoteRelationId,
		ruw:              ruw,
		changes:          changes,
		remoteUnitIds:    make(map[string]params.RemoteEntityId),
		facade:           facade,
		exportMutex:      exportMutex,
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
			logger.Debugf("relation units changed: %#v", change)
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
	unitNamesToExport := append(changedUnitNames, change.Departed...)
	remoteIds, err := w.ensureUnitsExported(unitNamesToExport)
	if err != nil {
		return nil, errors.Annotate(err, "exporting units")
	}

	// Construct the event to send to the remote model.
	event := &params.RemoteRelationChangeEvent{
		RelationId:    w.remoteRelationId,
		DepartedUnits: remoteIds[len(changedUnitNames):],
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
			remoteId := remoteIds[i]
			change := params.RemoteRelationUnitChange{
				UnitId:   remoteId,
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

func (w *relationUnitsWatcher) ensureUnitsExported(unitNames []string) ([]params.RemoteEntityId, error) {
	w.exportMutex.Lock()
	defer w.exportMutex.Unlock()

	var maybeUnexported []names.Tag
	for _, name := range unitNames {
		if _, ok := w.remoteUnitIds[name]; !ok {
			maybeUnexported = append(maybeUnexported, names.NewUnitTag(name))
		}
	}
	if len(maybeUnexported) > 0 {
		logger.Debugf("exporting units: %v", maybeUnexported)
		results, err := w.facade.ExportEntities(maybeUnexported)
		if err != nil {
			return nil, errors.Annotate(err, "exporting units")
		}
		for i, result := range results {
			if result.Error != nil && !params.IsCodeAlreadyExists(result.Error) {
				return nil, errors.Annotatef(result.Error, "exporting unit %q", maybeUnexported[i].Id())
			}
			w.remoteUnitIds[maybeUnexported[i].Id()] = *result.Result
		}
	}
	results := make([]params.RemoteEntityId, len(unitNames))
	for i, name := range unitNames {
		results[i] = w.remoteUnitIds[name]
	}
	return results, nil
}
