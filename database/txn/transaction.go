// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
)

// Logger describes methods for emitting log output.
type Logger interface {
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
	IsTraceEnabled() bool

	// Logf is used to proxy Dqlite logs via this logger.
	Logf(level loggo.Level, msg string, args ...interface{})
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
func WithLogger(logger Logger) Option {
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
	logger        Logger
	retryStrategy RetryStrategy
}

func newOptions() *option {
	logger := loggo.GetLogger("juju.database")
	return &option{
		timeout:       DefaultTimeout,
		logger:        logger,
		retryStrategy: defaultRetryStrategy(clock.WallClock, logger),
	}
}

// TransactionRunner defines a generic transactioner for applying transactions
// on a given database. It expects that no individual transaction function
// should take longer than the default timeout.
type TransactionRunner struct {
	timeout       time.Duration
	logger        Logger
	retryStrategy RetryStrategy
}

// NewTransactionRunner returns a new TransactionRunner.
func NewTransactionRunner(opts ...Option) *TransactionRunner {
	o := newOptions()
	for _, opt := range opts {
		opt(o)
	}

	return &TransactionRunner{
		timeout:       o.timeout,
		logger:        o.logger,
		retryStrategy: o.retryStrategy,
	}
}

// Txn defines a generic txn function for applying transactions on a given
// database. It expects that no individual transaction function should take
// longer than the default timeout.
// There are no retry semantics for running the function.
//
// This should not be used directly, instead the TrackedDB should be used to
// handle transactions.
func (t *TransactionRunner) Txn(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Trace(err)
	}

	if err := fn(ctx, tx); err != nil {
		if rErr := t.retryStrategy(ctx, tx.Rollback); rErr != nil {
			t.logger.Warningf("failed to rollback transaction: %v", rErr)
		}
		return errors.Trace(err)
	}

	if err := tx.Commit(); err != nil && err != sql.ErrTxDone {
		return errors.Trace(err)
	}

	return nil
}

// Retry defines a generic retry function for applying a function that
// interacts with the database. It will retry in cases of transient known
// database errors.
func (t *TransactionRunner) Retry(ctx context.Context, fn func() error) error {
	return t.retryStrategy(ctx, fn)
}

// defaultRetryStrategy returns a function that can be used to apply a default
// retry strategy to its input operation. It will retry in cases of transient
// known database errors.
func defaultRetryStrategy(clock clock.Clock, logger Logger) func(context.Context, func() error) error {
	return func(ctx context.Context, fn func() error) error {
		err := retry.Call(retry.CallArgs{
			Func: fn,
			IsFatalError: func(err error) bool {
				// No point in re-trying or logging a no-row error.
				if errors.Is(err, sql.ErrNoRows) {
					return true
				}

				// If the error is potentially retryable then keep going.
				if IsErrRetryable(err) {
					if logger.IsTraceEnabled() {
						logger.Tracef("retrying transaction: %v", err)
					}
					return false
				}

				return true
			},
			Attempts:    250,
			Delay:       time.Millisecond,
			MaxDelay:    time.Millisecond * 100,
			MaxDuration: time.Second * 25,
			BackoffFunc: retry.ExpBackoff(time.Millisecond, time.Millisecond*100, 0.8, true),
			Clock:       clock,
			Stop:        ctx.Done(),
		})
		return errors.Trace(err)
	}
}
