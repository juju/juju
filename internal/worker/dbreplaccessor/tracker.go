// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbreplaccessor

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	corecontext "github.com/juju/juju/core/context"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/database/pragma"
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

type trackedDBWorker struct {
	tomb tomb.Tomb

	dbApp     DBApp
	namespace string

	db    *sqlair.DB
	rawDB *sql.DB

	clock  clock.Clock
	logger logger.Logger
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
		dbApp:     dbApp,
		namespace: namespace,
		clock:     clock.WallClock,
	}

	for _, opt := range opts {
		opt(w)
	}

	db, err := w.dbApp.Open(ctx, w.namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(1)

	// Ensure that foreign keys are enabled, as we rely on them for
	// referential integrity.
	if err := pragma.SetPragma(ctx, db, pragma.ForeignKeysPragma, true); err != nil {
		return nil, errors.Annotate(err, "setting foreign keys pragma")
	}

	w.rawDB = db
	w.db = sqlair.NewDB(db)

	w.tomb.Go(w.loop)

	return w, nil
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
	// Tie the tomb to the context for the retry semantics.
	ctx = corecontext.WithSourceableError(w.tomb.Context(ctx), w)

	// Don't execute the function if we know the context is already done.
	if err := ctx.Err(); err != nil {
		return errors.Trace(err)
	}

	return fn(w.db)
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
	ctx, cancel := w.scopedContext()
	defer cancel()

	defer func() {
		err := w.rawDB.Close()
		if err != nil {
			w.logger.Errorf(ctx, "failed to close database: %v", err)
		}
	}()

	<-w.tomb.Dying()
	return tomb.ErrDying
}

func (w *trackedDBWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}
