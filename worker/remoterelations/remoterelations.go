// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
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
	// RegisterRemoteRelations sets up the remote model to participate
	// in the specified relations.
	RegisterRemoteRelations(relations ...params.RegisterRemoteRelationArg) ([]params.RegisterRemoteRelationResult, error)

	// PublishRelationChange publishes relation changes to the
	// model hosting the remote application involved in the relation.
	PublishRelationChange(params.RemoteRelationChangeEvent) error

	// WatchRelationUnits returns a watcher that notifies of changes to the
	// units in the remote model for the relation with the given remote token.
	WatchRelationUnits(arg params.RemoteEntityArg) (watcher.RelationUnitsWatcher, error)

	// RelationUnitSettings returns the relation unit settings for the given relation units in the remote model.
	RelationUnitSettings([]params.RemoteRelationUnit) ([]params.SettingsResult, error)

	// WatchRemoteApplicationRelations starts a RelationStatusWatcher for watching the
	// relations of each specified application in the remote model.
	WatchRelationStatus(arg params.RemoteEntityArg) (watcher.RelationStatusWatcher, error)
}

// RemoteRelationsFacade exposes remote relation functionality to a worker.
type RemoteRelationsFacade interface {
	// ImportRemoteEntity adds an entity to the remote entities collection
	// with the specified opaque token.
	ImportRemoteEntity(entity names.Tag, token string) error

	// SaveMacaroon saves the macaroon for the entity.
	SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error

	// ExportEntities allocates unique, remote entity IDs for the
	// given entities in the local model.
	ExportEntities([]names.Tag) ([]params.TokenResult, error)

	// GetToken returns the token associated with the entity with the given tag.
	GetToken(names.Tag) (string, error)

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
	// each specified application in the local model, and returns the watcher IDs
	// and initial values, or an error if the application's relations could not be
	// watched.
	WatchRemoteApplicationRelations(application string) (watcher.StringsWatcher, error)

	// ConsumeRemoteRelationChange consumes a change to settings originating
	// from the remote/offering side of a relation.
	ConsumeRemoteRelationChange(change params.RemoteRelationChangeEvent) error

	// ControllerAPIInfoForModel returns the controller api info for a model.
	ControllerAPIInfoForModel(modelUUID string) (*api.Info, error)
}

type newRemoteRelationsFacadeFunc func(*api.Info) (RemoteModelRelationsFacadeCloser, error)

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID                string
	RelationsFacade          RemoteRelationsFacade
	NewRemoteModelFacadeFunc newRemoteRelationsFacadeFunc
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
