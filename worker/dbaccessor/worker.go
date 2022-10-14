// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	"github.com/canonical/go-dqlite/app"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/database"
)

const replSocketFileName = "juju.sock"

// DBGetter describes the ability to supply a sql.DB
// reference for a particular database.
type DBGetter interface {
	// GetDB returns a sql.DB reference for the dqlite-backed database that
	// contains the data for the specified namespace.
	// A NotFound error is returned if the worker is unaware of the requested DB.
	GetDB(namespace string) (*sql.DB, error)
}

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	OptionFactory *database.OptionFactory
	Clock         clock.Clock
	Logger        Logger
	NewApp        func(string, ...app.Option) (DBApp, error)
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.OptionFactory == nil {
		return errors.NotValidf("missing Dqlite option factory")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	if c.NewApp == nil {
		return errors.NotValidf("missing NewApp")
	}
	return nil
}

type DBApp interface {
	// Open the dqlite database with the given name
	Open(context.Context, string) (*sql.DB, error)

	// Ready can be used to wait for a node to complete tasks that
	// are initiated at startup. For example a new node will attempt
	// to join the cluster, a restarted node will check if it should
	// assume some particular role, etc.
	//
	// If this method returns without error it means that those initial
	// tasks have succeeded and follow-up operations like Open() are more
	// likely to succeed quickly.
	Ready(context.Context) error

	// Handover transfers all responsibilities for this node (such has
	// leadership and voting rights) to another node, if one is available.
	//
	// This method should always be called before invoking Close(),
	// in order to gracefully shut down a node.
	Handover(context.Context) error

	// ID returns the dqlite ID of this application node.
	ID() uint64

	// Close the application node, releasing all resources it created.
	Close() error
}

type REPL interface {
	worker.Worker
}

type dbWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	mu        sync.RWMutex
	dbApp     DBApp
	dbReadyCh chan struct{}
	dbHandles map[string]*sql.DB
}

func newWorker(cfg WorkerConfig) (*dbWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &dbWorker{
		cfg:       cfg,
		dbHandles: make(map[string]*sql.DB),
		dbReadyCh: make(chan struct{}),
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *dbWorker) loop() (err error) {
	defer func() {
		w.mu.Lock()
		defer w.mu.Unlock()

		if w.dbApp == nil {
			return
		}

		if hErr := w.dbApp.Handover(context.TODO()); hErr != nil {
			if err == nil {
				err = errors.Annotate(err, "gracefully handing over dqlite node responsibilities")
			} else { // we are exiting with another error; so we just log this.
				w.cfg.Logger.Errorf("unable to gracefully hand off dqlite node responsibilities: %v", hErr)
			}
		}

		if cErr := w.dbApp.Close(); cErr != nil {
			if err == nil {
				err = errors.Annotate(cErr, "closing dqlite application instance")
			} else { // we are exiting with another error; so we just log this.
				w.cfg.Logger.Errorf("unable to close dqlite application instance: %v", cErr)
			}
		}
		w.dbApp = nil
	}()

	if err := w.initializeDqlite(); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *dbWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *dbWorker) Wait() error {
	return w.catacomb.Wait()
}

// GetDB returns a sql.DB reference for the dqlite-backed database that
// contains the data for the specified namespace.
// A NotFound error is returned if the worker is unaware of the requested DB.
// TODO (manadart 2022-10-14): At present, Dqlite does not expose the ability
// to detect if a database exists. so we can't just call `Open`
// (potentially creating new databases) without any guards.
// At this point, we expect any databases to have been opened and cached upon
// worker startup - if it isn't in the cache, it doesn't exist.
// Work out if we need an alternative approach later. We could lazily open and
// cache, but the access patterns are such that the DBs would all be opened
// pretty much at once anyway.
func (w *dbWorker) GetDB(namespace string) (*sql.DB, error) {
	select {
	case <-w.dbReadyCh:
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if cachedDB := w.dbHandles[namespace]; cachedDB != nil {
		return cachedDB, nil
	}

	return nil, errors.NotFoundf("database for model %q", namespace)
}

func (w *dbWorker) initializeDqlite() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.dbApp != nil {
		return nil
	}

	opt := w.cfg.OptionFactory

	dataDir, err := opt.EnsureDataDir()
	if err != nil {
		return errors.Trace(err)
	}

	withAddr, err := opt.WithAddressOption()
	if err != nil {
		return errors.Trace(err)
	}

	withTLS, err := opt.WithTLSOption()
	if err != nil {
		return errors.Trace(err)
	}

	withCluster, err := opt.WithClusterOption()
	if err != nil {
		return errors.Trace(err)
	}

	if w.dbApp, err = w.cfg.NewApp(dataDir, withAddr, withTLS, withCluster, opt.WithLogFuncOption()); err != nil {
		return errors.Trace(err)
	}

	// Ensure dqlite is ready to accept new changes.
	if err := w.dbApp.Ready(context.TODO()); err != nil {
		_ = w.dbApp.Close()
		w.dbApp = nil
		return errors.Annotatef(err, "ensuring Dqlite is ready to process changes")
	}

	if err := w.openDatabases(); err != nil {
		return errors.Annotate(err, "opening initial databases")
	}

	if err := w.startREPL(dataDir); err != nil {
		_ = w.dbApp.Close()
		w.dbApp = nil
		return errors.Annotatef(err, "starting Dqlite REPL")
	}

	w.cfg.Logger.Infof("initialized Dqlite application (ID: %v)", w.dbApp.ID())
	close(w.dbReadyCh) // start accepting GetDB() requests.
	return nil
}

func (w *dbWorker) openDatabases() error {
	db, err := w.dbApp.Open(context.TODO(), "controller")
	if err != nil {
		return errors.Trace(err)
	}

	w.dbHandles["controller"] = db
	return nil
}

func (w *dbWorker) startREPL(dataDir string) error {
	socketPath := filepath.Join(dataDir, replSocketFileName)
	_ = os.Remove(socketPath)

	repl, err := newREPL(socketPath, w, isRetryableError, w.cfg.Clock, w.cfg.Logger)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(w.catacomb.Add(repl))
}
