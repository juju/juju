// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"math/rand"
	"sync"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	corecontext "github.com/juju/juju/core/context"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/database/pragma"
	"github.com/juju/juju/internal/database/txn"
)

const (
	// States that report the state of the worker.
	stateStarted    = "started"
	stateDBReplaced = "db-replaced"
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
func WithLogger(logger logger.Logger) TrackedDBWorkerOption {
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
	internalStates chan string
	tomb           tomb.Tomb

	dbApp     DBApp
	namespace string

	mutex sync.RWMutex
	db    *sqlair.DB

	clock        clock.Clock
	logger       logger.Logger
	metrics      *Collector
	dbTxnMetrics txn.Metrics

	pingDBFunc func(context.Context, *sql.DB) error

	report *report
}

// NewTrackedDBWorker creates a new TrackedDBWorker
func NewTrackedDBWorker(
	ctx context.Context, dbApp DBApp, namespace string, opts ...TrackedDBWorkerOption,
) (TrackedDB, error) {
	return newTrackedDBWorker(ctx, nil, dbApp, namespace, opts...)
}

func newTrackedDBWorker(
	ctx context.Context, internalStates chan string,
	dbApp DBApp, namespace string,
	opts ...TrackedDBWorkerOption,
) (TrackedDB, error) {
	w := &trackedDBWorker{
		internalStates: internalStates,
		dbApp:          dbApp,
		namespace:      namespace,
		clock:          clock.WallClock,
		pingDBFunc:     defaultPingDBFunc,
		report:         &report{},
	}

	for _, opt := range opts {
		opt(w)
	}

	// Set the db transaction metrics for the namespace.
	w.dbTxnMetrics = w.metrics.DBMetricsForNamespace(namespace)

	db, err := w.dbApp.Open(ctx, w.namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Set the maximum number of idle and open connections to be the same and
	// set to 2 (default is 0 for MaxOpenConns). From testing, it's better to
	// have both set to the same value, and not setting these values can lead to
	// a large number of open connections being created and not closed, which
	// can lead to unbounded connections.
	//
	// If and when we change this number, be aware that a database will have 2
	// connections per database, per dqlite App. So if we have 100 databases
	// then that is 200 connections per dqlite App. Changing that number to
	// match runtime.GOMAXPROCS will then be len(database) * runtime.GOMAXPROCS
	// per dqlite App. This can lead to a lot of open connections, so be
	// careful.
	// If we ever move to sharding model databases across dqlite Apps,
	// then this number can be increased, as the number of connections per
	// dqlite App will be less because the number of databases per dqlite App
	// will be less. Testing will need to be done to determine the best number
	// for this.
	db.SetMaxIdleConns(2)
	db.SetMaxOpenConns(2)

	// Ensure that foreign keys are enabled, as we rely on them for
	// referential integrity.
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
		// Tie the worker tomb to the context, so that if the worker dies, we
		// can correctly kill the transaction via the context. The context will
		// now have the correct reason for the death of the transaction. Either
		// the tomb died or the context was cancelled.
		ctx = corecontext.WithSourceableError(w.tomb.Context(ctx), w)
		return errors.Trace(database.Txn(ctx, db, fn))
	})
}

// StdTxn executes the input function against the tracked database,
// within a transaction that depends on the input context.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
func (w *trackedDBWorker) StdTxn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return w.run(ctx, func(db *sqlair.DB) error {
		// Tie the worker tomb to the context, so that if the worker dies, we
		// can correctly kill the transaction via the context. The context will
		// now have the correct reason for the death of the transaction. Either
		// the tomb died or the context was cancelled.
		ctx = corecontext.WithSourceableError(w.tomb.Context(ctx), w)
		return errors.Trace(database.StdTxn(ctx, db.PlainDB(), fn))
	})
}

// Dying returns a channel that is closed when the database connection
// is no longer usable. This can be used to detect when the database is
// shutting down or has been closed.
func (w *trackedDBWorker) Dying() <-chan struct{} {
	return w.tomb.Dying()
}

// Err returns the error that caused the worker to stop.
func (w *trackedDBWorker) Err() error {
	return w.tomb.Err()
}

func (w *trackedDBWorker) run(ctx context.Context, fn func(*sqlair.DB) error) error {
	w.metrics.TxnRequests.WithLabelValues(w.namespace).Inc()

	// Tie the tomb to the context for the retry semantics.
	ctx = corecontext.WithSourceableError(w.tomb.Context(ctx), w)

	// Inject the metrics into the context for the txn.
	ctx = txn.WithMetrics(ctx, w.dbTxnMetrics)

	// Retry the so long as the tomb and the context are valid.
	return database.Retry(ctx, func() (err error) {
		begin := w.clock.Now()
		w.metrics.TxnRetries.WithLabelValues(w.namespace).Inc()
		w.metrics.DBRequests.WithLabelValues(w.namespace).Inc()
		defer w.meterDBOpResult(begin, err)

		// The underlying db could be swapped out if the database becomes
		// stale.
		w.mutex.RLock()
		db := w.db
		w.mutex.RUnlock()

		// If we ever get a nil database, then ensure we return an error,
		// rather than potentially causing panics down the line.
		if db == nil {
			return errors.NotFoundf("database")
		}

		// Don't execute the function if we know the context is already done.
		if err := ctx.Err(); err != nil {
			return errors.Trace(err)
		}

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

// KillWithReason kills the worker with a particular reason.
func (w *trackedDBWorker) KillWithReason(reason error) {
	w.tomb.Kill(reason)
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
	ctx, cancel := w.scopedContext()
	defer cancel()

	defer func() {
		w.mutex.Lock()
		defer w.mutex.Unlock()

		if w.db == nil {
			return
		}
		err := w.db.PlainDB().Close()
		if err != nil {
			w.logger.Debugf(ctx, "Closed database connection: %v", err)
		}
	}()

	// Report the initial started state.
	w.reportInternalState(stateStarted)

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

			newDB, err := w.ensureDBAliveAndOpenIfRequired(ctx, currentDB)
			if errors.Is(err, context.Canceled) {
				select {
				case <-w.tomb.Dying():
					return tomb.ErrDying
				default:
					return errors.Trace(err)
				}
			}
			if err != nil {
				// If we get an error, ensure we close the underlying db and
				// mark the tracked db in an error state.
				if err := currentDB.Close(); err != nil {
					w.logger.Errorf(ctx, "error closing database: %v", err)
				}

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
					w.logger.Errorf(ctx, "error closing database: %v", err)
				}
				w.db = sqlair.NewDB(newDB)
				w.report.Set(func(r *report) {
					r.dbReplacements++
				})
				w.mutex.Unlock()

				// Notify the internal state channel that the database has been
				// replaced.
				w.reportInternalState(stateDBReplaced)
			}

			timer.Reset(jitter(PollInterval, 0.1))
		}
	}
}

// ensureDBAliveAndOpenNewIfRequired is a bit long-winded, but it is a way to
// ensure that the underlying database is alive and well. If it is not, we
// attempt to open a new one. If that fails, we return an error.
func (w *trackedDBWorker) ensureDBAliveAndOpenIfRequired(ctx context.Context, db *sql.DB) (*sql.DB, error) {
	// Allow killing the tomb to cancel the context,
	// so shutdown/restart can not be blocked by this call.
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	if w.logger.IsLevelEnabled(logger.TRACE) {
		w.logger.Tracef(ctx, "ensuring database %q is alive", w.namespace)
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
			if w.logger.IsLevelEnabled(logger.TRACE) {
				w.logger.Tracef(ctx, "pinging database %q", w.namespace)
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
		w.logger.Warningf(ctx, "unable to ping database %q: attempting to reopen the database before trying again: %v",
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

func (w *trackedDBWorker) reportInternalState(state string) {
	select {
	case <-w.tomb.Dying():
	case w.internalStates <- state:
	default:
	}
}

func (w *trackedDBWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
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

// jitter returns a duration that is the input interval with a random factor
// applied to it. This prevents all workers from polling the database at the
// same time.
func jitter(interval time.Duration, factor float64) time.Duration {
	return time.Duration(float64(interval) * (1 + factor*(2*rand.Float64()-1)))
}
