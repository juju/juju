// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.remoterelations")

// RemoteApplicationsFacade exposes remote relation functionality to a worker.
type RemoteRelationsFacade interface {
	// ExportEntities allocates unique, remote entity IDs for the
	// given entities in the local model.
	ExportEntities([]names.Tag) ([]params.RemoteEntityIdResult, error)

	// PublishLocalRelationChange publishes local relation changes to the
	// model hosting the remote application involved in the relation.
	PublishLocalRelationChange(params.RemoteRelationsChange) error

	// RelationUnitSettings returns the relation unit settings for the
	// given relation units in the local model.
	RelationUnitSettings([]params.RelationUnit) ([]params.SettingsResult, error)

	// Relations returns information about the relations
	// with the specified keys in the local model.
	Relations(keys []string) ([]params.RelationResult, error)

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
	RelationsFacade RemoteRelationsFacade
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
	facade           RemoteRelationsFacade
}

type relation struct {
	params.RemoteRelationChange
	ruw *relationUnitsWatcher
}

func newRemoteApplicationWorker(
	relationsWatcher watcher.StringsWatcher,
	facade RemoteRelationsFacade,

) (worker.Worker, error) {
	w := &remoteApplicationWorker{
		relationsWatcher: relationsWatcher,
		facade:           facade,
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
				if err := w.relationChanged(key, result, relations); err != nil {
					return errors.Annotatef(err, "handling change for relation %q", key)
				}
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
	key string, result params.RelationResult, relations map[string]*relation,
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

	// If we have previously started the watcher and the
	// relation is now dead, stop the watcher.
	if r := relations[key]; r != nil {
		r.Life = result.Life
		if r.Life == params.Dead {
			return w.killRelationUnitWatcher(key, relations)
		}
		// Nothing to do, we have previously started the watcher.
		return nil
	}

	// Start a watcher to track changes to the local units in the
	// relation, and a worker to process those changes.
	if result.Life != params.Dead {
		localRelationUnitsWatcher, err := w.facade.WatchLocalRelationUnits(key)
		if err != nil {
			return errors.Trace(err)
		}
		relationUnitsWatcher, err := newRelationUnitsWatcher(
			names.NewRelationTag(key),
			localRelationUnitsWatcher,
			w.facade,
		)
		if err != nil {
			return errors.Trace(err)
		}
		if err := w.catacomb.Add(relationUnitsWatcher); err != nil {
			return errors.Trace(err)
		}
		r := &relation{}
		r.RelationId = result.Id
		r.Life = result.Life
		r.ruw = relationUnitsWatcher
		relations[key] = r
	}
	return nil
}

// relationUnitsWatcher uses a watcher.RelationUnitsWatcher to listen
// to changes to relation settings in the local model and converts
// to a params.RemoteRelationChanges for export to a remote model.
type relationUnitsWatcher struct {
	catacomb    catacomb.Catacomb
	relationTag names.RelationTag
	ruw         watcher.RelationUnitsWatcher
	facade      RemoteRelationsFacade
}

func newRelationUnitsWatcher(
	relationTag names.RelationTag,
	ruw watcher.RelationUnitsWatcher,
	facade RemoteRelationsFacade,
) (*relationUnitsWatcher, error) {
	w := &relationUnitsWatcher{
		relationTag: relationTag,
		ruw:         ruw,
		facade:      facade,
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
			if err := w.updateRelationUnitsChange(change); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (w *relationUnitsWatcher) updateRelationUnitsChange(
	change watcher.RelationUnitsChange,
) error {
	// TODO(wallyworld)
	return ObserverRelationUnitsChange(change)
}

// For testing only.
// TODO(wallyworld) - remove when more code added.
var ObserverRelationUnitsChange = func(change watcher.RelationUnitsChange) error {
	logger.Infof("relation units change: %v", change)
	return nil
}
