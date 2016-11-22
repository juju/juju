// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.remoterelations")

// RemoteApplicationsFacade exposes remote relation functionality to a worker.
type RemoteApplicationsFacade interface {
	// WatchLocalRelationUnits returns a watcher that notifies of changes to the
	// local units in the relation with the given key.
	WatchLocalRelationUnits(relationKey string) (watcher.RelationUnitsWatcher, error)

	// WatchRemoteApplications watches for addition, removal and lifecycle
	// changes to remote applications known to the local model.
	WatchRemoteApplications() (watcher.StringsWatcher, error)

	// WatchRemoteApplicationRelations starts a StringsWatcher for watching the relations of
	// each specified application in the local environment, and returns the watcher IDs
	// and initial values, or an error if the services' relations could not be
	// watched.
	WatchRemoteApplicationRelations(application string) (watcher.StringsWatcher, error)
}

// Config defines the operation of a Worker.
type Config struct {
	Facade RemoteApplicationsFacade
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.Facade == nil {
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
		config: config,
		logger: logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker manages relations and associated settings where
// one end of the relation is a remote application.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	logger   loggo.Logger
}

// Kill implements worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	for {
		// TODO(wallyworld)
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}
