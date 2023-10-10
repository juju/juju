// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
)

// BaseWatcher encapsulates members common to all EventQueue-based watchers.
// It has no functionality by itself, and is intended to be embedded in
// other more specific watchers.
type BaseWatcher struct {
	tomb tomb.Tomb

	watchableDB changestream.WatchableDB
	logger      Logger
}

// NewBaseWatcher returns a BaseWatcher constructed from the arguments.
func NewBaseWatcher(watchableDB changestream.WatchableDB, l Logger) *BaseWatcher {
	return &BaseWatcher{
		watchableDB: watchableDB,
		logger:      l,
	}
}

// Kill (worker.Worker) kills the watcher via its tomb.
func (w *BaseWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait (worker.Worker) waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *BaseWatcher) Wait() error {
	return w.tomb.Wait()
}

// Predicate is a function that determines whether a change event
// should be sent to the watcher.
// Returning false will prevent the events from being sent.
type Predicate func(context.Context, database.TxnRunner, []changestream.ChangeEvent) (bool, error)

// defaultPredicate is the default predicate used by the watchers.
// It will always return true, allowing all events to be sent.
func defaultPredicate(context.Context, database.TxnRunner, []changestream.ChangeEvent) (bool, error) {
	return true, nil
}
