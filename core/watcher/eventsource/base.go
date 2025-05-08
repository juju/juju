// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
)

// BaseWatcher encapsulates members common to all EventQueue-based watchers.
// It has no functionality by itself, and is intended to be embedded in
// other more specific watchers.
type BaseWatcher struct {
	tomb tomb.Tomb

	watchableDB changestream.WatchableDB
	logger      logger.Logger
}

// NewBaseWatcher returns a BaseWatcher constructed from the arguments.
func NewBaseWatcher(watchableDB changestream.WatchableDB, logger logger.Logger) *BaseWatcher {
	return &BaseWatcher{
		watchableDB: watchableDB,
		logger:      logger,
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

// Mapper is a function that maps a slice of change events to another slice
// of change events. This allows modification or dropping of events if
// necessary. When zero events returned, no change will be emitted.
// The inverse is also possible, allowing fake events to be added to the stream.
type Mapper func(context.Context, []changestream.ChangeEvent) ([]changestream.ChangeEvent, error)

// defaultMapper is the default mapper used by the watchers.
// It will always return the same change events, allowing all events to be sent.
func defaultMapper(
	_ context.Context, events []changestream.ChangeEvent,
) ([]changestream.ChangeEvent, error) {
	return events, nil
}

// FilterEvents drops events that do not match the filter.
func FilterEvents(filter func(changestream.ChangeEvent) bool) Mapper {
	return func(
		_ context.Context, events []changestream.ChangeEvent,
	) ([]changestream.ChangeEvent, error) {
		var filtered []changestream.ChangeEvent
		for _, event := range events {
			if filter(event) {
				filtered = append(filtered, event)
			}
		}
		return filtered, nil
	}
}

// Query is a function that returns the initial state of a watcher.
type Query[T any] func(context.Context, database.TxnRunner) (T, error)
