// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"gopkg.in/tomb.v2"

	coredb "github.com/juju/juju/core/db"
	"github.com/juju/juju/database/app"
)

const (
	// PollInterval is the amount of time to wait between polling the database.
	PollInterval = time.Second * 30

	// DefaultVerifyAttempts is the number of attempts to verify the database,
	// by opening a new database on verification failure.
	DefaultVerifyAttempts = 3
)

// TrackedDB defines the union of a TrackedDB and a worker.Worker interface.
// This is local to the package, allowing for better testing of the underlying
// trackerDB worker.
type TrackedDB interface {
	coredb.TrackedDB
	worker.Worker
}

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
	GetDB(namespace string) (coredb.TrackedDB, error)
}

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	NodeManager       NodeManager
	Clock             clock.Clock
	Logger            Logger
	NewApp            func(string, ...app.Option) (DBApp, error)
	NewDBWorker       func(DBApp, string, FatalErrorChecker, clock.Clock, Logger) (TrackedDB, error)
	FatalErrorChecker FatalErrorChecker
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
	if c.NewDBWorker == nil {
		return errors.NotValidf("missing NewDBWorker")
	}
	if c.FatalErrorChecker == nil {
		return errors.NotValidf("missing FatalErrorChecker")
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
	dbRunner  *worker.Runner
}

func newWorker(cfg WorkerConfig) (*dbWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &dbWorker{
		cfg:       cfg,
		dbReadyCh: make(chan struct{}),
		dbRunner: worker.NewRunner(worker.RunnerParams{
			Clock: cfg.Clock,
			// TODO (stickupkid): For the initial concept, we don't want to
			// bring down other workers when one goes down. In the future we
			// can look into custom errors to allow that.
			IsFatal:      func(err error) bool { return false },
			RestartDelay: time.Second * 30,
		}),
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.dbRunner,
		},
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

		ctx, cancel := context.WithTimeout(context.TODO(), time.Second*30)
		defer cancel()

		if hErr := w.dbApp.Handover(ctx); hErr != nil {
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
func (w *dbWorker) GetDB(namespace string) (coredb.TrackedDB, error) {
	select {
	case <-w.dbReadyCh:
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	if e, err := w.dbRunner.Worker(namespace, w.catacomb.Dying()); err == nil {
		return e.(coredb.TrackedDB), nil
	}

	trackedDB, err := w.openDatabase(namespace)
	return trackedDB, errors.Trace(err)
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

	// Open up the default controller database. Other database namespaces can
	// be opened up in a more lazy fashion.
	if _, err := w.openDatabase("controller"); err != nil {
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

func (w *dbWorker) openDatabase(namespace string) (coredb.TrackedDB, error) {
	var trackedDB coredb.TrackedDB
	if err := w.dbRunner.StartWorker(namespace, func() (worker.Worker, error) {
		dbWorker, err := w.cfg.NewDBWorker(w.dbApp, namespace, w.cfg.FatalErrorChecker, w.cfg.Clock, w.cfg.Logger)
		if err != nil {
			return nil, errors.Trace(err)
		}
		trackedDB = dbWorker
		return dbWorker, nil
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return trackedDB, nil
}

func (w *dbWorker) startREPL(dataDir string) error {
	socketPath := filepath.Join(dataDir, replSocketFileName)
	_ = os.Remove(socketPath)

	repl, err := newREPL(socketPath, w, w.cfg.Clock, w.cfg.Logger)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(w.catacomb.Add(repl))
}

type trackedDBWorker struct {
	tomb tomb.Tomb

	dbApp     DBApp
	namespace string

	mutex   sync.RWMutex
	db      *sql.DB
	err     error
	isFatal FatalErrorChecker

	preparesMutex sync.RWMutex
	prepares      map[uint64]func(*sql.DB) error
	preparesIdx   uint64

	clock  clock.Clock
	logger Logger
}

// NewTrackedDBWorker creates a new TrackedDBWorker
func NewTrackedDBWorker(dbApp DBApp, namespace string, isFatal FatalErrorChecker, clock clock.Clock, logger Logger) (TrackedDB, error) {
	w := &trackedDBWorker{
		dbApp:     dbApp,
		namespace: namespace,
		isFatal:   isFatal,
		clock:     clock,
		logger:    logger,
		prepares:  make(map[uint64]func(*sql.DB) error),
	}

	var err error
	w.db, err = w.dbApp.Open(context.TODO(), w.namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w.tomb.Go(w.loop)

	return w, nil
}

// DB closes over a raw *sql.DB. Closing over the DB allows the late
// realization of the database. Allowing retries of DB acquistion if there
// is a failure that is non-retryable.
func (w *trackedDBWorker) DB(fn func(*sql.DB) error) error {
	w.mutex.RLock()
	// We have a fatal error, the DB can not be accessed.
	if w.err != nil {
		w.mutex.RUnlock()
		return errors.Trace(w.err)
	}
	db := w.db
	w.mutex.RUnlock()

	return fn(db)
}

// Txn closes over a raw *sql.Tx. This allows retry semantics in only one
// location. For instances where the underlying sql database is busy or if
// it's a common retryable error that can be handled cleanly in one place.
func (w *trackedDBWorker) Txn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return w.DB(func(db *sql.DB) error {
		return coredb.Retry(func() error {
			return coredb.Txn(ctx, db, fn)
		})
	})
}

// PrepareStmts prepares a given set of statements for a given db instance.
// If the db is changed this will be recalled. To prevent this, the first
// result unregisters the statements.
func (w *trackedDBWorker) PrepareStmts(fn func(*sql.DB) error) (func(), error) {
	w.preparesMutex.Lock()
	w.preparesIdx++
	w.prepares[w.preparesIdx] = fn
	w.preparesMutex.Unlock()

	if err := w.DB(fn); err != nil {
		return nil, errors.Trace(err)
	}

	return func() {
		w.preparesMutex.Lock()
		defer w.preparesMutex.Unlock()

		delete(w.prepares, w.preparesIdx)
	}, nil
}

// Err will return an blocking errors from the underlying tracking source.
func (w *trackedDBWorker) Err() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	return w.err
}

// Kill implements worker.Worker
func (w *trackedDBWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker
func (w *trackedDBWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *trackedDBWorker) loop() error {
	timer := w.clock.NewTimer(PollInterval)
	defer timer.Stop()

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-timer.Chan():
			// Any retryable errors are handled at the txn level. If we get an
			// error returning here, we've either exhausted the number of
			// retries or the error was fatal.
			if err := w.verifyDB(); err != nil {
				// If we get an error, ensure we close the underlying db and
				// mark the tracked db in an error state.
				w.mutex.Lock()
				w.db.Close()
				w.err = errors.Trace(err)
				w.mutex.Unlock()

				// As we failed attempting to verify the db, we're in a fatal
				// state. Collapse the worker and if required, cause the other
				// workers to follow suite.
				return errors.Trace(err)
			}
		}
	}
}

func (w *trackedDBWorker) verifyDB() error {
	// Force the timeout to be lower that the DefaultTimeout, so we can spot
	// issues a lot quicker.
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
	defer cancel()

	if w.logger.IsTraceEnabled() {
		w.logger.Tracef("verify db: querying sqlite_schema for number of tables")
	}

	w.mutex.RLock()
	db := w.db
	w.mutex.RUnlock()

	// There are multiple levels of retries here. For good reason. We want to
	// retry the transaction semantics for retryable errors. Then if that fails
	// we want to retry if the database is at fault. In that case we want
	// to open up a new database and try the transaction again.
	var (
		err         error
		substituted bool
	)
	for i := 0; i < DefaultVerifyAttempts; i++ {
		// Verify that we don't have a potential nil database from the retry
		// semantics.
		if db == nil {
			return errors.Errorf("no database found")
		}

		err = coredb.Retry(func() error {
			return coredb.Txn(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
				row := tx.QueryRowContext(ctx, "SELECT COUNT(tbl_name) FROM sqlite_schema")

				var tableCount int
				if err := row.Scan(&tableCount); err != nil {
					return errors.Trace(err)
				}

				if w.logger.IsTraceEnabled() {
					w.logger.Tracef("verify db: number of tables within the db", tableCount)
				}
				return errors.Trace(row.Err())
			})
		})
		// We were successful at requesting the schema, so we can bail out
		// early.
		if err == nil {
			// If the database isn't the same one we started with, then force
			// the new one into the worker.
			if substituted {
				w.mutex.Lock()
				w.db = db
				w.mutex.Unlock()

				if err := w.runPrepareStmts(); err != nil {
					// We failed to prepare statements. We know this is a fatal
					// error, so just crash the worker.
					return errors.Trace(err)
				}
			}

			return nil
		}

		// We failed to apply the transaction, so just return the error and
		// cause the worker to crash.
		if i == DefaultVerifyAttempts-1 {
			return errors.Trace(err)
		}

		// We got an error that is non-retryable, attempt to open a new database
		// connection and see if that works.
		w.logger.Errorf("verify db: unable to query db, attempting to open the database before applying txn")

		// Attempt to open a new database. If there is an error, just crash
		// the worker, we can't do anything else.
		db, err = w.dbApp.Open(context.TODO(), w.namespace)
		if err != nil {
			return errors.Trace(err)
		}

		substituted = true
	}
	return errors.Trace(err)
}

func (w *trackedDBWorker) runPrepareStmts() error {
	// Copy the prepared statements to a slice, so any attempts to unregister
	// whilst preparing won't cause issues.
	w.preparesMutex.RLock()
	var prepares []func(*sql.DB) error
	for _, s := range w.prepares {
		prepares = append(prepares, s)
	}
	w.preparesMutex.RUnlock()

	err := w.DB(func(db *sql.DB) error {
		// TODO (stickupkid): Implement retry semantics for prepared statements.
		for _, fn := range prepares {
			if err := fn(db); err != nil {
				w.logger.Errorf("unable to prepare statements: %v", err)
			}
		}
		return nil
	})
	return errors.Trace(err)
}
