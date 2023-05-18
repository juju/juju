// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
)

// NotifyWatcher watches for changes to a database table.
type NotifyWatcher struct {
	*BaseWatcher

	out       chan struct{}
	tableName string
	selectAll string
}

// NewNotifyWatcher returns a new watcher that receives changes ...
func NewNotifyWatcher(base *BaseWatcher, tableName, scope string) *NotifyWatcher {
	w := &NotifyWatcher{
		BaseWatcher: base,
		out:         make(chan struct{}),
		tableName:   tableName,
		selectAll:   fmt.Sprintf("SELECT %s FROM %s", scope, tableName),
	}

	w.tomb.Go(w.loop)
	return w
}

// Changes returns the channel on which the scopes for
// changed rows are sent to downstream consumers.
func (w *NotifyWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *NotifyWatcher) loop() error {
	subscription, err := w.eventQueue.Subscribe(changestream.Namespace(w.tableName, changestream.All))
	if err != nil {
		return errors.Annotatef(err, "subscribing to namespace %q", w.tableName)
	}
	defer subscription.Unsubscribe()

	initialState, err := w.getInitialState()
	if err != nil {
		return errors.Annotate(err, "retrieving initial watcher state")
	}

	// By reassigning the in and out channels, we effectively ticktock between
	// read mode and dispatch mode. This ensures we always dispatch deltas that
	// we received before reading more, and every channel read/write is guarded
	// by checks of the tomb and subscription liveness.
	// We begin in dispatch mode in order to send the initial state.
	var (
		changes struct{}

		out chan<- struct{}
		in  <-chan []changestream.ChangeEvent
	)
	if len(initialState) > 0 {
		changes = struct{}{}
		out = w.out
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-subscription.Done():
			return ErrSubscriptionClosed
		case subChanges, ok := <-in:
			if !ok {
				w.logger.Debugf("change channel closed for %q; terminating watcher", w.tableName)
				return nil
			}

			// We have changes. Tick over to dispatch mode.
			if len(subChanges) > 0 {
				changes = struct{}{}

				in = nil
				out = w.out
			}
		case out <- changes:
			// We have dispatched. Tick over to read mode.
			in = subscription.Changes()
			out = nil
		}
	}
}

// getInitialState retrieves the current state of the world from the database,
// as it concerns this watcher. It must be called after we are subscribed.
// Note that killing the worker via its tomb cancels the context used here.
func (w *NotifyWatcher) getInitialState() ([]string, error) {
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var scopes []string
	err := w.db.Txn(w.tomb.Context(parentCtx), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, w.selectAll)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return errors.Trace(err)
		}
		defer func() { _ = rows.Close() }()

		for i := 0; rows.Next(); i++ {
			scopes = append(scopes, "")
			if err := rows.Scan(&scopes[i]); err != nil {
				return errors.Trace(err)
			}
		}

		if err := rows.Err(); err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(rows.Close())
	})

	return scopes, errors.Trace(err)
}

// Kill (worker.Worker) kills the watcher via its tomb.
func (w *NotifyWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait (worker.Worker) waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *NotifyWatcher) Wait() error {
	return w.tomb.Wait()
}
