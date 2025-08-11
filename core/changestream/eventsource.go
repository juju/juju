// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/errors"
)

// EventSource describes the ability to subscribe
// to a subset of events from a change stream.
type EventSource interface {
	// Subscribe returns a subscription that can receive events from
	// a change stream according to the input subscription options.
	Subscribe(summary string, opts ...SubscriptionOption) (Subscription, error)
}

// WatchableDB describes the ability to run transactions against a database
// and to subscribe to events emitted from that same source.
type WatchableDB interface {
	database.TxnRunner
	EventSource
}

// WatchableDBGetter describes the ability to get
// a WatchableDB for a particular namespace.
type WatchableDBGetter interface {
	GetWatchableDB(string) (WatchableDB, error)
}

// NewTxnRunnerFactory returns a TxnRunnerFactory for the input
// changestream.WatchableDB factory function.
// This ensures that we never pass the ability to access the
// change-stream into a state object.
// State objects should only be concerned with persistence and retrieval.
// Watchers are the concern of the service layer.
func NewTxnRunnerFactory(f WatchableDBFactory) database.TxnRunnerFactory {
	return func() (database.TxnRunner, error) {
		r, err := f()
		return r, errors.Capture(err)
	}
}

// WatchableDBFactory provides a function for getting a database.TxnRunner or
// an error.
type WatchableDBFactory = func() (WatchableDB, error)

// NewWatchableDBFactoryForNamespace returns a WatchableDBFactory
// for the input namespaced factory function and namespace.
func NewWatchableDBFactoryForNamespace[T WatchableDB](f func(string) (T, error), ns string) WatchableDBFactory {
	return func() (WatchableDB, error) {
		r, err := f(ns)
		return r, errors.Capture(err)
	}
}
