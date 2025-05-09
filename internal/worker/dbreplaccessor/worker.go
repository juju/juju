// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbreplaccessor

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// errTryAgain indicates that the worker should try
	// again later to start a DB tracker worker.
	errTryAgain = errors.ConstError("DB node is nil, but worker is not dying; rescheduling TrackedDB start attempt")
)

// NodeManager creates Dqlite `App` initialisation arguments and options.
type NodeManager interface {
	// EnsureDataDir ensures that a directory for Dqlite data exists at
	// a path determined by the agent config, then returns that path.
	EnsureDataDir() (string, error)

	// DqliteSQLDriver returns a Dqlite SQL driver that can be used to
	// connect to the Dqlite cluster. This is a read only connection, which is
	// intended to be used for running queries against the Dqlite cluster (REPL).
	DqliteSQLDriver(ctx context.Context) (driver.Driver, error)
}

// DBGetter describes the ability to supply a sql.DB
// reference for a particular database.
type DBGetter interface {
	// GetDB returns a sql.DB reference for the dqlite-backed database that
	// contains the data for the specified namespace.
	// A NotFound error is returned if the worker is unaware of the requested DB.
	GetDB(namespace string) (database.TxnRunner, error)
}

// dbRequest is used to pass requests for TrackedDB
// instances into the worker loop.
type dbRequest struct {
	namespace string
	done      chan error
}

// makeDBGetRequest creates a new TrackedDB request for the input namespace.
func makeDBGetRequest(namespace string) dbRequest {
	return dbRequest{
		namespace: namespace,
		done:      make(chan error),
	}
}

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	NodeManager NodeManager
	Clock       clock.Clock

	Logger          logger.Logger
	NewApp          NewAppFunc
	NewDBReplWorker NewDBReplWorkerFunc
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.NodeManager == nil {
		return errors.NotValidf("missing NodeManager")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.NewApp == nil {
		return errors.NotValidf("missing NewApp")
	}
	if c.NewDBReplWorker == nil {
		return errors.NotValidf("missing NewDBReplWorker")
	}
	return nil
}

type dbReplWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	dbApp        DBApp
	dbReplRunner *worker.Runner
	driverName   string

	// dbReplReady is used to signal that we can
	// begin processing GetDB requests.
	dbReplReady chan struct{}

	// dbReplRequests is used to synchronise GetDB
	// requests into this worker's event loop.
	dbReplRequests chan dbRequest
}

// NewWorker creates a new dbaccessor worker.
func NewWorker(cfg WorkerConfig) (*dbReplWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:  "dbreplaccessor",
		Clock: cfg.Clock,
		// If a worker goes down, we've attempted multiple retries and in
		// that case we do want to cause the dbaccessor to go down. This
		// will then bring up a new dqlite app.
		IsFatal: func(err error) bool {
			// If a database is dead we should not kill the worker of the
			// runner.
			if errors.Is(err, database.ErrDBDead) {
				return false
			}

			// If there is a rebind during starting up a worker the dbApp
			// will be nil. In this case, we'll return ErrTryAgain. In this
			// case we don't want to kill the worker. We'll force the
			// worker to try again.
			return !errors.Is(err, errTryAgain)
		},
		ShouldRestart: func(err error) bool {
			return !errors.Is(err, database.ErrDBDead)
		},
		RestartDelay: time.Second * 1,
		Logger:       internalworker.WrapLogger(cfg.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &dbReplWorker{
		cfg:            cfg,
		dbReplRunner:   runner,
		dbReplReady:    make(chan struct{}),
		dbReplRequests: make(chan dbRequest),
		driverName:     fmt.Sprintf("dqlite-%d", 1000+rand.IntN(1000000)),
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Name: "db-repl-accessor",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.dbReplRunner,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *dbReplWorker) loop() (err error) {
	if err := w.joinExistingDqliteCluster(); err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	for {
		select {
		// The following ensures that all dbReplRequests are serialised and
		// processed in order.
		case req := <-w.dbReplRequests:
			if err := w.openDatabase(ctx, req.namespace); err != nil {
				select {
				case req.done <- errors.Annotatef(err, "opening database for namespace %q", req.namespace):
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				}
				continue
			}

			req.done <- nil

		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *dbReplWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *dbReplWorker) Wait() error {
	return w.catacomb.Wait()
}

// GetDB returns a transaction runner for the dqlite-backed
// database that contains the data for the specified namespace.
func (w *dbReplWorker) GetDB(namespace string) (database.TxnRunner, error) {
	// Ensure Dqlite is initialised.
	select {
	case <-w.dbReplReady:
	case <-w.catacomb.Dying():
		return nil, database.ErrDBReplAccessorDying
	}

	// First check if we've already got the db worker already running.
	// The runner is effectively a DB cache.
	if db, err := w.workerFromCache(namespace); err != nil {
		if errors.Is(err, w.catacomb.ErrDying()) {
			return nil, database.ErrDBReplAccessorDying
		}

		return nil, errors.Trace(err)
	} else if db != nil {
		return db, nil
	}

	// Enqueue the request as it's either starting up and we need to wait longer
	// or it's not running and we need to start it.
	req := makeDBGetRequest(namespace)
	select {
	case w.dbReplRequests <- req:
	case <-w.catacomb.Dying():
		return nil, database.ErrDBReplAccessorDying
	}

	// Wait for the worker loop to indicate it's done.
	select {
	case err := <-req.done:
		// If we know we've got an error, just return that error before
		// attempting to ask the dbReplRunnerWorker.
		if err != nil {
			return nil, errors.Trace(err)
		}
	case <-w.catacomb.Dying():
		return nil, database.ErrDBReplAccessorDying
	}

	// This will return a not found error if the request was not honoured.
	// The error will be logged - we don't crash this worker for bad calls.
	tracked, err := w.dbReplRunner.Worker(namespace, w.catacomb.Dying())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return tracked.(database.TxnRunner), nil
}

func (w *dbReplWorker) workerFromCache(namespace string) (database.TxnRunner, error) {
	// If the worker already exists, return the existing worker early.
	if tracked, err := w.dbReplRunner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return tracked.(database.TxnRunner), nil
	} else if errors.Is(errors.Cause(err), worker.ErrDead) {
		// Handle the case where the DB runner is dead due to this worker dying.
		select {
		case <-w.catacomb.Dying():
			return nil, w.catacomb.ErrDying()
		default:
			return nil, errors.Trace(err)
		}
	} else if !errors.Is(errors.Cause(err), errors.NotFound) {
		// If it's not a NotFound error, return the underlying error.
		// We should only start a worker if it doesn't exist yet.
		return nil, errors.Trace(err)
	}

	// We didn't find the worker. Let the caller decide what to do.
	return nil, nil
}

// joinExistingDqliteCluster takes care of joining an existing dqlite cluster.
func (w *dbReplWorker) joinExistingDqliteCluster() error {
	mgr := w.cfg.NodeManager
	if _, err := mgr.EnsureDataDir(); err != nil {
		return errors.Annotatef(err, "ensuring data directory")
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	driver, err := mgr.DqliteSQLDriver(ctx)
	if err != nil {
		return errors.Annotatef(err, "creating dqlite driver")
	}

	sql.Register(w.driverName, driver)

	w.dbApp, err = w.cfg.NewApp(w.driverName)
	if err != nil {
		return errors.Trace(err)
	}

	// Begin handling external requests.
	close(w.dbReplReady)

	return nil
}

// openDatabase starts a TrackedDB worker for the database with the input name.
// It is called by initialiseDqlite to open the controller databases,
// and via GetDB to service downstream database requests.
// It is important to note that the start function passed to StartWorker is not
// invoked synchronously.
// Since GetDB blocks until dbReplReady is closed, and initialiseDqlite waits for
// the node to be ready, we can assume that we will never race with a nil dbApp
// when first starting up.
// Since the only way we can get into this race is during shutdown or a rebind,
// it is safe to return ErrDying if the catacomb is dying when we detect a nil
// database or ErrTryAgain to force the runner to retry starting the worker
// again.
func (w *dbReplWorker) openDatabase(ctx context.Context, namespace string) error {
	// Note: Do not be tempted to create the worker outside of the StartWorker
	// function. This will create potential data race if openDatabase is called
	// multiple times for the same namespace.
	err := w.dbReplRunner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		if w.dbApp == nil {
			// If the dbApp is nil then we're shutting down.
			select {
			case <-w.catacomb.Dying():
				return nil, w.catacomb.ErrDying()
			default:
				return nil, errTryAgain
			}
		}

		return w.cfg.NewDBReplWorker(ctx,
			w.dbApp, namespace,
			WithClock(w.cfg.Clock),
			WithLogger(w.cfg.Logger),
		)
	})
	if errors.Is(err, errors.AlreadyExists) {
		return nil
	}
	return errors.Trace(err)
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *dbReplWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}
