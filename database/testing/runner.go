// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database/txn"
)

var defaultTransactionRunner = txn.NewRetryingTxnRunner()

// trackedDB is used for testing purposes.
type txnRunner struct {
	db *sqlair.DB
}

// Txn executes the input function against the tracked database, using
// the sqlair package. The sqlair package provides a mapping library for
// SQL queries and statements.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
func (t *txnRunner) Txn(ctx context.Context, fn func(context.Context, *sqlair.TX) error) error {
	return errors.Trace(defaultTransactionRunner.Txn(ctx, t.db, fn))
}

// StdTxn executes the input function against the tracked database,
// within a transaction that depends on the input context.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
func (t *txnRunner) StdTxn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return defaultTransactionRunner.Retry(ctx, func() error {
		return errors.Trace(defaultTransactionRunner.StdTxn(ctx, t.db.PlainDB(), fn))
	})
}

// TxnRunnerFactory returns a DBFactory that returns the given database.
func TxnRunnerFactory(db coredatabase.TxnRunner) func() (coredatabase.TxnRunner, error) {
	return func() (coredatabase.TxnRunner, error) {
		if db == nil {
			return nil, errors.New("nil db")
		}
		return db, nil
	}
}

type singularDBGetter struct {
	runner coredatabase.TxnRunner
}

func (s singularDBGetter) GetDB(name string) (coredatabase.TxnRunner, error) {
	return s.runner, nil
}

// SingularDBGetter returns a DBGetter that always returns the given database.
func SingularDBGetter(runner coredatabase.TxnRunner) coredatabase.DBGetter {
	return singularDBGetter{
		runner: runner,
	}
}

// ConstFactory returns a changestream.WatchableDB factory function from just a
// database.TxnRunner.
func ConstFactory(runner coredatabase.TxnRunner) func() (changestream.WatchableDB, error) {
	return func() (changestream.WatchableDB, error) {
		return constWatchableDB{
			TxnRunner: runner,
		}, nil
	}
}

type constWatchableDB struct {
	coredatabase.TxnRunner
	changestream.EventSource
}

// Subscribe returns a subscription that can receive events from
// a change stream according to the input subscription options.
func (constWatchableDB) Subscribe(opts ...changestream.SubscriptionOption) (changestream.Subscription, error) {
	return constSubscription{}, nil
}

type constSubscription struct{}

// Changes returns the channel that the subscription will receive events on.
func (constSubscription) Changes() <-chan []changestream.ChangeEvent {
	return make(<-chan []changestream.ChangeEvent)
}

// Unsubscribe removes the subscription from the event queue.
func (constSubscription) Unsubscribe() {}

// Done provides a way to know from the consumer side if the underlying
// subscription has been terminated. This is useful to know if the
// event queue has been killed.
func (constSubscription) Done() <-chan struct{} {
	return make(<-chan struct{})
}

// StubWatchableDB is a watchable db with a events channel
// that can be used in tests.
type StubWatchableDB struct {
	coredatabase.TxnRunner
	Events chan []changestream.ChangeEvent
}

// Subscribe returns a subscription that can receive events from
// a change stream according to the input subscription options.
func (s *StubWatchableDB) Subscribe(opts ...changestream.SubscriptionOption) (changestream.Subscription, error) {
	return &stubSubscription{events: s.Events}, nil
}

type stubSubscription struct {
	events chan []changestream.ChangeEvent
}

// Changes returns the channel that the subscription will receive events on.
func (s *stubSubscription) Changes() <-chan []changestream.ChangeEvent {
	return s.events
}

// Unsubscribe removes the subscription from the event queue.
func (*stubSubscription) Unsubscribe() {}

// Done provides a way to know from the consumer side if the underlying
// subscription has been terminated. This is useful to know if the
// event queue has been killed.
func (*stubSubscription) Done() <-chan struct{} {
	return make(<-chan struct{})
}
