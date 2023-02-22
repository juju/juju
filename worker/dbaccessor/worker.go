// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/database/app"
)

const replSocketFileName = "juju.sock"

// NodeManager creates Dqlite `App` initialisation arguments and options.
type NodeManager interface {
	// IsExistingNode returns true if this machine of container has run a
	// Dqlite node in the past
	IsExistingNode() (bool, error)

	// EnsureDataDir ensures that a directory for Dqlite data exists at
	// a path determined by the agent config, then returns that path.
	EnsureDataDir() (string, error)

	// WithLogFuncOption returns a Dqlite application Option that will proxy Dqlite
	// log output via this factory's logger where the level is recognised.
	WithLogFuncOption() app.Option

	// WithAddressOption returns a Dqlite application Option
	// for specifying the local address:port to use.
	WithAddressOption() (app.Option, error)

	// WithTLSOption returns a Dqlite application Option for TLS encryption
	// of traffic between clients and clustered application nodes.
	WithTLSOption() (app.Option, error)

	// WithClusterOption returns a Dqlite application Option for initialising
	// Dqlite as the member of a cluster with peers representing other controllers.
	WithClusterOption() (app.Option, error)
}

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
	NodeManager NodeManager
	Clock       clock.Clock
	Logger      Logger
	NewApp      func(string, ...app.Option) (DBApp, error)
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.NodeManager == nil {
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

	mgr := w.cfg.NodeManager

	dataDir, err := mgr.EnsureDataDir()
	if err != nil {
		return errors.Trace(err)
	}

	options := []app.Option{mgr.WithLogFuncOption()}

	// If we have started this node before, there is no need to supply address
	// or clustering options. These are already committed to Dqlite's Raft log
	// and are indicated by the contents of cluster.yaml and info.yaml in the
	// data directory.
	extant, err := mgr.IsExistingNode()
	if err != nil {
		return errors.Trace(err)
	}

	if extant {
		w.cfg.Logger.Infof("host is configured as a Dqlite node")
	} else {
		w.cfg.Logger.Infof("configuring host as a Dqlite node for the first time")

		withAddr, err := mgr.WithAddressOption()
		if err != nil {
			return errors.Trace(err)
		}
		options = append(options, withAddr)

		withCluster, err := mgr.WithClusterOption()
		if err != nil {
			return errors.Trace(err)
		}
		options = append(options, withCluster)
	}

	if w.dbApp, err = w.cfg.NewApp(dataDir, options...); err != nil {
		return errors.Trace(err)
	}

	// Ensure Dqlite is ready to accept new changes.
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

	// Start handling GetDB calls from third parties.
	close(w.dbReadyCh)
	w.cfg.Logger.Infof("initialized Dqlite application (ID: %v)", w.dbApp.ID())
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
