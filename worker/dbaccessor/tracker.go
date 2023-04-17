// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database"
)

const (
	// PollInterval is the amount of time to wait between polling the database.
	PollInterval = time.Second * 10

	// DefaultVerifyAttempts is the number of attempts to verify the database,
	// by opening a new database on verification failure.
	DefaultVerifyAttempts = 3
)

// TrackedDB defines the union of a TrackedDB and a worker.Worker interface.
// This is local to the package, allowing for better testing of the underlying
// trackerDB worker.
type TrackedDB interface {
	coredatabase.TrackedDB
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
	db    *sql.DB
	err   error

	clock   clock.Clock
	logger  Logger
	metrics *Collector

	pingDBFunc func(context.Context, *sql.DB) error
}

// NewTrackedDBWorker creates a new TrackedDBWorker
func NewTrackedDBWorker(dbApp DBApp, namespace string, opts ...TrackedDBWorkerOption) (TrackedDB, error) {
	w := &trackedDBWorker{
		dbApp:      dbApp,
		namespace:  namespace,
		clock:      clock.WallClock,
		pingDBFunc: defaultPingDBFunc,
	}

	for _, opt := range opts {
		opt(w)
	}

	var err error
	w.db, err = w.dbApp.Open(context.TODO(), w.namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w.tomb.Go(w.loop)

	return w, nil
}

// Txn executes the input function against the tracked database,
// within a transaction that depends on the input context.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
func (w *trackedDBWorker) Txn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return database.Retry(ctx, func() error {
		return errors.Trace(w.TxnNoRetry(ctx, fn))
	})
}

// TxnNoRetry executes the input function against the tracked database,
// within a transaction that depends on the input context.
// We meter both the total transaction count and active operations.
func (w *trackedDBWorker) TxnNoRetry(ctx context.Context, fn func(context.Context, *sql.Tx) error) (err error) {
	begin := w.clock.Now()
	w.metrics.TxnRequests.WithLabelValues(w.namespace).Inc()
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

	return errors.Trace(database.Txn(ctx, db, fn))
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

// Err will return any fatal errors that have occurred on the worker, trying
// to acquire the database.
func (w *trackedDBWorker) Err() error {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

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
			w.mutex.RLock()
			currentDB := w.db
			w.mutex.RUnlock()

			newDB, err := w.ensureDBAliveAndOpenIfRequired(currentDB)
			if err != nil {
				// If we get an error, ensure we close the underlying db and
				// mark the tracked db in an error state.
				w.mutex.Lock()
				if err := w.db.Close(); err != nil {
					w.logger.Errorf("error closing database: %v", err)
				}
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
				if err := w.db.Close(); err != nil {
					w.logger.Errorf("error closing database: %v", err)
				}
				w.db = newDB
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
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*10)
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

		err := database.Retry(ctx, func() error {
			if w.logger.IsTraceEnabled() {
				w.logger.Tracef("pinging database %q", w.namespace)
			}
			return w.pingDBFunc(ctx, db)
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
	}
	return nil, errors.NotValidf("database")
}

func defaultPingDBFunc(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}
