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

type DBGetter interface {
	GetDB(namespace string) (*sql.DB, error)
	GetExistingDB(namespace string) (*sql.DB, error)
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

func NewWorker(cfg WorkerConfig) (*dbWorker, error) {
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

// GetDB returns a sql.DB handle for the dqlite-backed database that contains
// the data for the specified namespace.
func (w *dbWorker) GetDB(namespace string) (*sql.DB, error) {
	select {
	case <-w.dbReadyCh:
		// dqlite has been initialized
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if a DB handle has already been created and cached.
	if cachedDB := w.dbHandles[namespace]; cachedDB != nil {
		return cachedDB, nil
	}

	// Create and cache DB instance.
	db, err := w.dbApp.Open(context.TODO(), namespace)
	if err != nil {
		return nil, errors.Annotatef(err, "accessing DB for namespace %q", namespace)
	}
	w.dbHandles[namespace] = db
	return db, nil
}

// GetExistingDB returns a sql.DB handle for the dqlite-backed database that
// contains the data for the specified namespace or a NotFound error if the
// DB is not known.
func (w *dbWorker) GetExistingDB(namespace string) (*sql.DB, error) {
	select {
	case <-w.dbReadyCh:
		// dqlite has been initialized
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if a DB handle has already been created and cached.
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
		return errors.Annotatef(err, "ensuring dqlite is ready to process changes")
	}

	if err := w.startREPL(dataDir); err != nil {
		_ = w.dbApp.Close()
		w.dbApp = nil
		return errors.Annotatef(err, "starting dqlite REPL")
	}

	w.cfg.Logger.Infof("initialized dqlite application (ID: %v)", w.dbApp.ID())
	close(w.dbReadyCh) // start accepting GetDB() requests.
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
