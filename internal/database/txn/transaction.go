// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/canonical/go-dqlite/v2/tracing"
	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/database/drivererrors"
	internallogger "github.com/juju/juju/internal/logger"
)

// committable represents a transaction interface that can be used for
// committing a transaction.
type committable interface {
	Commit() error
}

const (
	DefaultTimeout = time.Second * 30
)

// RetryStrategy defines a function for retrying a transaction.
type RetryStrategy func(context.Context, func() error) error

// Option defines a function for setting options on a TransactionRunner.
type Option func(*option)

// WithTimeout defines a timeout for the transaction. This is useful for
// defining a timeout for a transaction that is expected to take longer than
// the default timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(o *option) {
		o.timeout = timeout
	}
}

// WithLogger defines a logger for the transaction.
func WithLogger(logger logger.Logger) Option {
	return func(o *option) {
		o.logger = logger
	}
}

// WithRetryStrategy defines a retry strategy for the transaction.
func WithRetryStrategy(retryStrategy RetryStrategy) Option {
	return func(o *option) {
		o.retryStrategy = retryStrategy
	}
}

type option struct {
	timeout       time.Duration
	logger        logger.Logger
	retryStrategy RetryStrategy
}

func newOptions() *option {
	logger := internallogger.GetLogger("juju.database")
	return &option{
		timeout:       DefaultTimeout,
		logger:        logger,
		retryStrategy: DefaultRetryStrategy(clock.WallClock, logger),
	}
}

// RetryingTxnRunner defines a generic runner for applying transactions
// to a given database. It expects that no individual transaction function
// should take longer than the default timeout.
// Transient errors are retried based on the defined retry strategy.
type RetryingTxnRunner struct {
	timeout       time.Duration
	logger        logger.Logger
	retryStrategy RetryStrategy
	tracePool     sync.Pool
	loggerPool    sync.Pool
	txnID         uint64
}

// NewRetryingTxnRunner returns a new RetryingTxnRunner.
func NewRetryingTxnRunner(opts ...Option) *RetryingTxnRunner {
	o := newOptions()
	for _, opt := range opts {
		opt(o)
	}

	// Create one span pool up front, so all pooled tracing can use the
	// same one.
	spanPool := &sync.Pool{
		New: func() any {
			return &dqliteSpan{}
		},
	}

	return &RetryingTxnRunner{
		timeout:       o.timeout,
		logger:        o.logger,
		retryStrategy: o.retryStrategy,

		tracePool: sync.Pool{
			New: func() any {
				return &dqliteTracer{
					pool:   spanPool,
					logger: o.logger,
				}
			},
		},
		loggerPool: sync.Pool{
			New: func() any {
				return &logTracer{
					logger: o.logger,
				}
			},
		},
	}
}

// Txn executes the input function against the tracked database, using
// the sqlair package. The sqlair package provides a mapping library for
// SQL queries and statements.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
//
// This should not be used directly, instead the TxnRunner should be used to
// handle transactions.
func (t *RetryingTxnRunner) Txn(ctx context.Context, db *sqlair.DB, fn func(context.Context, *sqlair.TX) error) error {
	return t.run(ctx, func(ctx context.Context) error {
		tx, err := db.Begin(ctx, nil)
		if err != nil {
			return errors.Trace(err)
		}

		if err := fn(ctx, tx); err != nil {
			if rErr := t.retryStrategy(ctx, tx.Rollback); rErr != nil {
				t.logger.Warningf(ctx, "failed to rollback transaction: %v", rErr)
			}
			return errors.Trace(err)
		}

		return errors.Trace(t.commit(ctx, tx))
	})
}

// StdTxn executes the input function against the tracked database,
// within a transaction that depends on the input context.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
//
// This should not be used directly, instead the TxnRunner should be used to
// handle transactions.
func (t *RetryingTxnRunner) StdTxn(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) error {
	return t.run(ctx, func(ctx context.Context) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return errors.Trace(err)
		}

		if err := fn(ctx, tx); err != nil {
			if rErr := t.retryStrategy(ctx, tx.Rollback); rErr != nil {
				t.logger.Warningf(ctx, "failed to rollback transaction: %v", rErr)
			}
			return errors.Trace(err)
		}

		return errors.Trace(t.commit(ctx, tx))
	})
}

// Commit is split out as we can't pass a context directly to the commit. To
// enable tracing, we need to just wrap the commit call. All other traces are
// done at the dqlite level.
func (t *RetryingTxnRunner) commit(ctx context.Context, tx committable) (err error) {
	if t.logger.IsLevelEnabled(logger.TRACE) {
		t.logger.Tracef(ctx, "running txn (id: %d) with query: COMMIT", ctx.Value(txnIDKey))
	}

	// Hardcode the name of the span
	_, span := trace.Start(ctx, traceName("commit"))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := tx.Commit(); err != nil && err != sql.ErrTxDone {
		return errors.Trace(err)
	}
	return nil
}

// Retry defines a generic retry function for applying a function that
// interacts with the database. It will retry in cases of transient known
// database errors.
func (t *RetryingTxnRunner) Retry(ctx context.Context, fn func() error) error {
	return t.retryStrategy(ctx, fn)
}

// run will execute the input function if there is a semaphore slot available.
func (t *RetryingTxnRunner) run(ctx context.Context, fn func(context.Context) error) (err error) {
	// If the context is already done then return early to prevent doing any
	// work.
	if err := ctx.Err(); err != nil {
		return errors.Trace(err)
	}

	ctx, span := trace.Start(ctx, traceName("run"))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// Txn ID is used to track the whole transaction, from the start to the end.
	// This also includes the BEGIN, COMMIT and ROLLBACK.
	// As these are processed in real-time, it will require the user of the
	// logging infrastructure to be able to track the txnID and to patch the
	// log lines together. The alternative is to use the tracing infrastructure
	// which will give it a context, but that's a lot of overhead to run in
	// production for every query.
	txnID := atomic.AddUint64(&t.txnID, 1)

	// Put the txnID onto the context, so we can the retrieve it later on for
	// the commit transaction logging. Without it, we'll have BEGIN, queries,
	// but no COMMIT.
	ctx = context.WithValue(ctx, txnIDKey, txnID)

	// This is the last generic place that we can place a trace for the
	// dqlite library. Ideally we would push this into the dqlite only code,
	// but that requires a lot of abstractions, that I'm unsure is worth it.
	var queryable queryable
	if tracer, enabled := trace.TracerFromContext(ctx); enabled {
		dtracer := t.tracePool.Get().(*dqliteTracer)
		defer t.tracePool.Put(dtracer)

		dtracer.prepare(tracer, txnID)

		ctx = tracing.WithTracer(ctx, dtracer)

		queryable = dtracer
	} else {
		// If the logger is trace enabled, then we can use the log tracer. The
		// log tracer is a light weight tracer that just logs the query and the
		// txnID. This is useful for debugging.
		ltrace := t.loggerPool.Get().(*logTracer)
		defer t.loggerPool.Put(ltrace)

		ltrace.prepare(txnID)

		ctx = tracing.WithTracer(ctx, ltrace)

		queryable = ltrace
	}

	err = fn(ctx)
	if err == nil {
		return nil
	}

	// If there is any constraint error then we should log it as an error.
	if drivererrors.IsConstraintError(err) {
		t.logger.Errorf(ctx, "constraint error %v - running queries:\n %v", err, queryable.Queries())
	}

	return errors.Trace(err)
}

// DefaultRetryStrategy returns a function that can be used to apply a default
// retry strategy to its input operation. It will retry in cases of transient
// known database errors.
func DefaultRetryStrategy(clock clock.Clock, log logger.Logger) func(context.Context, func() error) error {
	return func(ctx context.Context, fn func() error) error {
		metrics := MetricsFromContext(ctx)
		err := retry.Call(retry.CallArgs{
			Func: func() error {
				err := fn()

				// Record the success if there is no error.
				if err == nil {
					metrics.RecordSuccess()
					return nil
				}

				// Recording of the error is done in the IsFatalError function.
				return errors.Trace(err)
			},
			IsFatalError: func(err error) bool {
				// No point in re-trying or logging a no-row error.
				if errors.Is(err, sql.ErrNoRows) {
					metrics.RecordError(noRowsError)
					return true
				}

				// If the error is potentially retryable then keep going.
				if drivererrors.IsErrRetryable(err) {
					// Record the error for the metrics. We could potentially
					// record the error type here, but it's not clear what
					// value that would provide.
					metrics.RecordError(retryableError)

					if log.IsLevelEnabled(logger.TRACE) {
						log.Tracef(ctx, "retrying transaction: %v", err)
					}
					return false
				}

				metrics.RecordError(unknownError)
				return true
			},
			Attempts:    250,
			Delay:       time.Millisecond,
			MaxDelay:    time.Millisecond * 100,
			MaxDuration: time.Second * 25,
			BackoffFunc: retry.ExpBackoff(time.Millisecond, time.Millisecond*100, 1.2, true),
			Clock:       clock,
			Stop:        ctx.Done(),
		})
		return err
	}
}

const (
	// rootTraceName is used to define the root trace name for all transaction
	// traces.
	// This is purely for optimization purposes, as we can't use the
	// trace.NameFromFunc for all these micro traces.
	rootTraceName = "txn.(*RetryingTxnRunner)."
)

func traceName(name string) trace.Name {
	return trace.Name(rootTraceName + name)
}

type queryable interface {
	Queries() string
}

// logTracer is a pooled object for implementing a log tracing from a
// per-transaction trace. This works by piggy backing off the OTEL tracing
// implementation in go-dqlite package. The OTEL tracing already exposes the
// query, we can create a log tracer that just logs the query and the txnID.
type logTracer struct {
	logger logger.Logger

	mu           sync.Mutex
	txnID        uint64
	traceEnabled bool
	queries      []string
}

func (d *logTracer) prepare(txnID uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.txnID = txnID
	d.queries = d.queries[:0]
	d.traceEnabled = d.logger.IsLevelEnabled(logger.TRACE)
}

func (d *logTracer) Start(ctx context.Context, name string, query string) (context.Context, tracing.Span) {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Log the start of the transaction.
	if d.traceEnabled {
		d.logger.Tracef(ctx, "running txn (id: %d) with query: %s", d.txnID, query)
	}

	// This is less than ideal, it might be better to bulk create an array
	// of strings (maybe 256) as a ballast and then wipe them out when preparing
	// them. We could then just insert them into the array, rather than
	// appending them.
	d.queries = append(d.queries, query)

	return ctx, d
}

func (d *logTracer) End() {}

// Queries returns the queries that have been run in the transaction.
func (d *logTracer) Queries() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return strings.Join(d.queries, "\n")
}

// dqliteTracer is a pooled object for implementing a dqlite tracing from a
// juju tracing trace. The dqlite trace is just the lightest touch for
// implementing tracing. The library doesn't need to include the full OTEL
// library, it's overkill. In doing so, it has a reduced tracing API.
// As there are going to thousands of these in flight, it's wise to pool these
// as much as possible, to provide compatibility.
type dqliteTracer struct {
	logger logger.Logger

	tracer trace.Tracer
	pool   *sync.Pool

	mu      sync.Mutex
	txnID   uint64
	queries []string
}

func (d *dqliteTracer) prepare(tracer trace.Tracer, txnID uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.txnID = txnID
	d.queries = d.queries[:0]
	d.tracer = tracer
}

// Start creates a span and a context.Context containing the newly-created
// span.
func (d *dqliteTracer) Start(ctx context.Context, name string, query string) (context.Context, tracing.Span) {
	// Log the start of the transaction.
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logger.Tracef(ctx, "running txn (id: %d) with query: %s", d.txnID, query)

	// Start the span.
	ctx, span := d.tracer.Start(ctx, name, trace.WithAttributes(trace.StringAttr("query", query)))

	// Track the event, so it's possible to tie this back to the logs.
	span.AddEvent("txn", trace.Int64Attr("id", int64(d.txnID)))

	dspan := d.pool.Get().(*dqliteSpan)
	defer d.pool.Put(dspan)

	// Force the span onto the pooled object. We guarantee that the span
	// should be done once the run has been completed.
	dspan.span = span

	// This is less than ideal, it might be better to bulk create an array
	// of strings (maybe 256) as a ballast and then wipe them out when preparing
	// them. We could then just insert them into the array, rather than
	// appending them.
	d.queries = append(d.queries, query)

	return ctx, dspan
}

// Queries returns the queries that have been run in the transaction.
func (d *dqliteTracer) Queries() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return strings.Join(d.queries, "\n")
}

type dqliteSpan struct {
	span trace.Span
}

// End completes the Span. The Span is considered complete and ready to be
// delivered through the rest of the telemetry pipeline after this method
// is called. Therefore, updates to the Span are not allowed after this
// method has been called.
func (d *dqliteSpan) End() {
	d.span.End()
}
