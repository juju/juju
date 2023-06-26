// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
)

// NamespaceWatcher watches for changes to a database table.
// Any time rows change in the watched table, the changed
// values from the column specified by keyName are emitted.
type NamespaceWatcher struct {
	*BaseWatcher

	out chan []string

	// TODO (manadart 2023-05-24): Consider making this plural (composite key)
	// if/when it is supported by the change log table structure and stream.
	keyName    string
	tableName  string
	selectAll  string
	changeMask changestream.ChangeType
}

// NewUUIDsWatcher is a convenience method for creating a new
// NamespaceWatcher for the "uuid" column of the input table name.
func NewUUIDsWatcher(base *BaseWatcher, changeMask changestream.ChangeType, tableName string) watcher.StringsWatcher {
	return NewNamespaceWatcher(base, changeMask, tableName, "uuid")
}

// NewNamespaceWatcher returns a new watcher that receives changes from the
// input base watcher's db/queue when rows in the input table change.
func NewNamespaceWatcher(
	base *BaseWatcher, changeMask changestream.ChangeType, tableName, keyName string,
) watcher.StringsWatcher {
	w := &NamespaceWatcher{
		BaseWatcher: base,
		out:         make(chan []string),
		tableName:   tableName,
		keyName:     keyName,
		selectAll:   fmt.Sprintf("SELECT %s FROM %s", keyName, tableName),
		changeMask:  changeMask,
	}

	w.tomb.Go(w.loop)
	return w
}

// Changes returns the channel on which the keys for
// changed rows are sent to downstream consumers.
func (w *NamespaceWatcher) Changes() <-chan []string {
	return w.out
}

func (w *NamespaceWatcher) loop() error {
	defer close(w.out)

	if w.changeMask == 0 {
		return errors.NotValidf("changeMask value: 0")
	}
	subscription, err := w.watchableDB.Subscribe(changestream.Namespace(w.tableName, w.changeMask))
	if err != nil {
		return errors.Annotatef(err, "subscribing to namespace %q", w.tableName)
	}
	defer subscription.Unsubscribe()

	changes, err := w.getInitialState()
	if err != nil {
		return errors.Annotatef(
			err, "retrieving initial watcher state for namespace %q and key %q", w.tableName, w.keyName)
	}

	// By reassigning the in and out channels, we effectively ticktock between
	// read mode and dispatch mode. This ensures we always dispatch deltas that
	// we received before reading more, and every channel read/write is guarded
	// by checks of the tomb and subscription liveness.
	// We begin in dispatch mode in order to send the initial state.
	var in <-chan []changestream.ChangeEvent
	out := w.out

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
			changes = transform.Slice(subChanges, func(c changestream.ChangeEvent) string { return c.Changed() })
			in = nil
			out = w.out
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
func (w *NamespaceWatcher) getInitialState() ([]string, error) {
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var keys []string
	err := w.watchableDB.StdTxn(w.tomb.Context(parentCtx), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, w.selectAll)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return errors.Trace(err)
		}
		defer func() { _ = rows.Close() }()

		for i := 0; rows.Next(); i++ {
			var key string
			if err := rows.Scan(&key); err != nil {
				return errors.Trace(err)
			}
			keys = append(keys, key)
		}

		if err := rows.Err(); err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(rows.Close())
	})

	return keys, errors.Trace(err)
}

// Kill (worker.Worker) kills the watcher via its tomb.
func (w *NamespaceWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait (worker.Worker) waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *NamespaceWatcher) Wait() error {
	return w.tomb.Wait()
}
