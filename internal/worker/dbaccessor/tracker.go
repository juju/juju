// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/database/pragma"
)

const (
	// PollInterval is the amount of time to wait between polling the database.
	PollInterval = time.Second * 10

	// DefaultVerifyAttempts is the number of attempts to verify the database,
	// by opening a new database on verification failure.
	DefaultVerifyAttempts = 3
)

// TrackedDB defines the union of a TxnRunner and a worker.Worker interface.
// This is local to the package, allowing for better testing of the underlying
// trackedDB worker.
type TrackedDB interface {
	coredatabase.TxnRunner
	worker.Worker
}

// TrackedDBWorkerOption is a function that configures a TrackedDBWorker.
type TrackedDBWorkerOption func(*trackedDBWorker)

// WithPingDBFunc sets the function used to verify the database connection.
func WithPingDBFunc(f func(context.Context, *sql.DB) error) TrackedDBWorkerOption {
	return func(w *trackedDBWorker) {
		w.pingDBFunc = f
	}
}

// WithClock sets the clock used by the worker.
func WithClock(clock clock.Clock) TrackedDBWorkerOption {
	return func(w *trackedDBWorker) {
		w.clock = clock
	}
}

// WithLogger sets the logger used by the worker.
func WithLogger(logger Logger) TrackedDBWorkerOption {
	return func(w *trackedDBWorker) {
		w.logger = logger
	}
}

// WithMetricsCollector sets the metrics collector used by the worker.
func WithMetricsCollector(metrics *Collector) TrackedDBWorkerOption {
	return func(w *trackedDBWorker) {
		w.metrics = metrics
	}
}

type trackedDBWorker struct {
	tomb tomb.Tomb

	dbApp     DBApp
	namespace string

	mutex sync.RWMutex
	db    *sqlair.DB
	err   error

	clock   clock.Clock
	logger  Logger
	metrics *Collector

	pingDBFunc func(context.Context, *sql.DB) error

	report *report
}

// NewTrackedDBWorker creates a new TrackedDBWorker
func NewTrackedDBWorker(
	ctx context.Context, dbApp DBApp, namespace string, opts ...TrackedDBWorkerOption,
) (TrackedDB, error) {
	w := &trackedDBWorker{
		dbApp:      dbApp,
		namespace:  namespace,
		clock:      clock.WallClock,
		pingDBFunc: defaultPingDBFunc,
		report:     &report{},
	}

	for _, opt := range opts {
		opt(w)
	}

	db, err := w.dbApp.Open(ctx, w.namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := pragma.SetPragma(ctx, db, pragma.ForeignKeysPragma, true); err != nil {
		return nil, errors.Annotate(err, "setting foreign keys pragma")
	}

	w.db = sqlair.NewDB(db)

	// This logic must be performed here and not in the parent worker,
	// because we must ensure it occurs before the worker is considered
	// started. This prevents calls to GetDB for the same namespace
	// racing with the application of schema DDL.
	if namespace != coredatabase.ControllerNS {
		if err := w.ensureModelDBInitialised(ctx); err != nil {
			return nil, errors.Trace(err)
		}
	}

	w.tomb.Go(w.loop)

	return w, nil
}

func (w *trackedDBWorker) ensureModelDBInitialised(ctx context.Context) error {
	// Check if the DB has metadata for one of our known tables.
	var runDBMigration bool
	if err := errors.Trace(w.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE name='change_log'")

		var dummy int
		if err := row.Scan(&dummy); err == nil {
			// This database has the schema applied.
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return errors.Trace(err)
		}

		// We need to run the migration.
		runDBMigration = true
		return nil
	})); err != nil {
		return errors.Trace(err)
	}

	if !runDBMigration {
		return nil
	}

	return errors.Trace(database.NewDBMigration(w, w.logger, schema.ModelDDL()).Apply(ctx))
}

// Txn executes the input function against the tracked database,
// within a transaction that depends on the input context.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
func (w *trackedDBWorker) Txn(ctx context.Context, fn func(context.Context, *sqlair.TX) error) error {
	return w.run(ctx, func(db *sqlair.DB) error {
		return errors.Trace(database.Txn(w.tomb.Context(ctx), db, fn))
	})
}

// StdTxn executes the input function against the tracked database,
// within a transaction that depends on the input context.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
func (w *trackedDBWorker) StdTxn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return w.run(ctx, func(db *sqlair.DB) error {
		return errors.Trace(database.StdTxn(w.tomb.Context(ctx), db.PlainDB(), fn))
	})
}

func (w *trackedDBWorker) run(ctx context.Context, fn func(*sqlair.DB) error) error {
	w.metrics.TxnRequests.WithLabelValues(w.namespace).Inc()
	return database.Retry(w.tomb.Context(ctx), func() (err error) {
		begin := w.clock.Now()
		w.metrics.TxnRetries.WithLabelValues(w.namespace).Inc()
		w.metrics.DBRequests.WithLabelValues(w.namespace).Inc()
		defer w.meterDBOpResult(begin, err)

		// If the DB health check failed, the worker's error will be set,
		// and we will be without a usable database reference. Return the error.
		w.mutex.RLock()
		if w.err != nil {
			w.mutex.RUnlock()
			return errors.Trace(w.err)
		}

		db := w.db
		w.mutex.RUnlock()

		return fn(db)
	})
}

// meterDBOpResults decrements the active DB operation count,
// and records the result and duration of the completed operation.
func (w *trackedDBWorker) meterDBOpResult(begin time.Time, err error) {
	w.metrics.DBRequests.WithLabelValues(w.namespace).Dec()
	result := "success"
	if err != nil {
		result = "error"
	}
	w.metrics.DBDuration.WithLabelValues(w.namespace, result).Observe(w.clock.Now().Sub(begin).Seconds())
}

// Kill implements worker.Worker
func (w *trackedDBWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker
func (w *trackedDBWorker) Wait() error {
	return w.tomb.Wait()
}

// Report provides information for the engine report.
func (w *trackedDBWorker) Report() map[string]any {
	return w.report.Report()
}

func (w *trackedDBWorker) loop() error {
	defer func() {
		w.mutex.Lock()
		defer w.mutex.Unlock()

		if w.db == nil {
			return
		}
		err := w.db.PlainDB().Close()
		if err != nil {
			w.logger.Debugf("Closed database connection: %v", err)
		}
	}()

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
			w.mutex.RLock()
			currentDB := w.db.PlainDB()
			w.mutex.RUnlock()

			newDB, err := w.ensureDBAliveAndOpenIfRequired(currentDB)
			if err != nil {
				// If we get an error, ensure we close the underlying db and
				// mark the tracked db in an error state.
				if err := currentDB.Close(); err != nil {
					w.logger.Errorf("error closing database: %v", err)
				}
				w.mutex.Lock()
				w.err = errors.Trace(err)
				w.mutex.Unlock()

				// As we failed attempting to verify the db, we're in a fatal
				// state. Collapse the worker and if required, cause the other
				// workers to follow suite.
				return errors.Trace(err)
			}

			// We've got a new DB. Close the old one and replace it with the
			// new one, if they're not the same.
			if newDB != currentDB {
				w.mutex.Lock()
				if err := currentDB.Close(); err != nil {
					w.logger.Errorf("error closing database: %v", err)
				}
				w.db = sqlair.NewDB(newDB)
				w.report.Set(func(r *report) {
					r.dbReplacements++
				})
				w.err = nil
				w.mutex.Unlock()
			}

			timer.Reset(PollInterval)
		}
	}
}

// ensureDBAliveAndOpenNewIfRequired is a bit long-winded, but it is a way to
// ensure that the underlying database is alive and well. If it is not, we
// attempt to open a new one. If that fails, we return an error.
func (w *trackedDBWorker) ensureDBAliveAndOpenIfRequired(db *sql.DB) (*sql.DB, error) {
	// Allow killing the tomb to cancel the context,
	// so shutdown/restart can not be blocked by this call.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	ctx = w.tomb.Context(ctx)
	defer cancel()

	if w.logger.IsTraceEnabled() {
		w.logger.Tracef("ensuring database %q is alive", w.namespace)
	}

	// There are multiple levels of retries here.
	// - We want to retry the ping function for retryable errors.
	//   These might be DB-locked or busy-syncing errors for example.
	// - If the error is fatal, we discard the DB instance and reconnect
	//   before attempting health verification again.
	for i := 0; i < DefaultVerifyAttempts; i++ {
		// Verify that we don't have a potential nil database from the retry
		// semantics.
		if db == nil {
			return nil, errors.NotFoundf("database")
		}

		// Record the total ping.
		pingStart := w.clock.Now()
		var pingAttempts uint32 = 0
		err := database.Retry(ctx, func() error {
			if w.logger.IsTraceEnabled() {
				w.logger.Tracef("pinging database %q", w.namespace)
			}
			pingAttempts++
			return w.pingDBFunc(ctx, db)
		})
		pingDur := w.clock.Now().Sub(pingStart)

		// Record the ping attempt and duration.
		w.report.Set(func(r *report) {
			r.pingAttempts = pingAttempts
			r.pingDuration = pingDur
			if pingDur > r.maxPingDuration {
				r.maxPingDuration = pingDur
			}
		})

		// We were successful at requesting the schema, so we can bail out
		// early.
		if err == nil {
			return db, nil
		}

		// We exhausted the retry strategy for pinging the database.
		// Terminate the worker with the error.
		if i == DefaultVerifyAttempts-1 {
			return nil, errors.Trace(err)
		}

		// We got an error that is non-retryable, attempt to open a new database
		// connection and see if that works.
		w.logger.Warningf("unable to ping database %q: attempting to reopen the database before trying again: %v",
			w.namespace, err)

		// Attempt to open a new database. If there is an error, just crash
		// the worker, we can't do anything else.
		if db, err = w.dbApp.Open(ctx, w.namespace); err != nil {
			return nil, errors.Trace(err)
		}

		if err := pragma.SetPragma(ctx, db, pragma.ForeignKeysPragma, true); err != nil {
			return nil, errors.Annotate(err, "setting foreign keys pragma")
		}
	}
	return nil, errors.NotValidf("database")
}

func defaultPingDBFunc(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}

// report fields for the engine report.
type report struct {
	sync.Mutex

	// pingDuration is the duration of the last ping.
	pingDuration time.Duration
	// pingAttempts is the number of attempts to ping the database for the
	// last ping.
	pingAttempts uint32
	// maxPingDuration is the maximum duration of a ping for a given lifetime
	// of the worker.
	maxPingDuration time.Duration
	// dbReplacements is the number of times the database has been replaced
	// due to a failed ping.
	dbReplacements uint32
}

// Report provides information for the engine report.
func (r *report) Report() map[string]any {
	r.Lock()
	defer r.Unlock()

	return map[string]any{
		"last-ping-duration": r.pingDuration.String(),
		"last-ping-attempts": r.pingAttempts,
		"max-ping-duration":  r.maxPingDuration.String(),
		"db-replacements":    r.dbReplacements,
	}
}

// Set allows to set the report fields, guarded by a mutex.
func (r *report) Set(f func(*report)) {
	r.Lock()
	f(r)
	r.Unlock()
}
