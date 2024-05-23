// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"
	"database/sql"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher"
)

// NamespaceQuery is a function that returns the initial state of a
// namespace watcher.
type NamespaceQuery Query[[]string]

// NamespaceWatcher watches for changes in a namespace.
// Any time events matching the change mask occur in the namespace,
// the values associated with the events are emitted.
type NamespaceWatcher struct {
	*BaseWatcher

	out chan []string

	// TODO (manadart 2023-05-24): Consider making this plural (composite key)
	// if/when it is supported by the change log table structure and stream.
	namespace    string
	initialQuery NamespaceQuery
	changeMask   changestream.ChangeType

	mapper Mapper
}

// NewNamespaceWatcher returns a new watcher that receives changes from the
// input base watcher's db/queue when changes in the namespace occur.
func NewNamespaceWatcher(
	base *BaseWatcher, namespace string,
	changeMask changestream.ChangeType, initialQuery NamespaceQuery,
) watcher.StringsWatcher {
	w := &NamespaceWatcher{
		BaseWatcher:  base,
		out:          make(chan []string),
		namespace:    namespace,
		initialQuery: initialQuery,
		changeMask:   changeMask,
		mapper:       defaultMapper,
	}

	w.tomb.Go(w.loop)
	return w
}

// NewNamespaceMapperWatcher returns a new watcher that receives changes
// from the input base watcher's db/queue when changes in the namespace occur.
func NewNamespaceMapperWatcher(
	base *BaseWatcher, namespace string,
	changeMask changestream.ChangeType, initialQuery NamespaceQuery,
	mapper Mapper,
) watcher.StringsWatcher {
	w := &NamespaceWatcher{
		BaseWatcher:  base,
		out:          make(chan []string),
		namespace:    namespace,
		initialQuery: initialQuery,
		changeMask:   changeMask,
		mapper:       mapper,
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
	subscription, err := w.watchableDB.Subscribe(changestream.Namespace(w.namespace, w.changeMask))
	if err != nil {
		return errors.Annotatef(err, "subscribing to namespace %q", w.namespace)
	}
	defer subscription.Unsubscribe()

	ctx := w.tomb.Context(context.Background())

	changes, err := w.initialQuery(ctx, w.watchableDB)
	if err != nil {
		return errors.Annotatef(
			err, "retrieving initial watcher state for namespace %q", w.namespace)
	}

	// By reassigning the in and out channels, we effectively ticktock between
	// read mode and dispatch mode. This ensures we always dispatch deltas that
	// we received before reading more, and every channel read/write is guarded
	// by checks of the tomb and subscription liveness.
	// We begin in dispatch mode in order to send the initial state.
	var in <-chan []changestream.ChangeEvent
	out := w.out

	// Note: we don't use the mappper to prevent the initial event. All
	// namespace watchers are __required__ to send the initial state. The API
	// design for watchers when they subscribe is that they must send the
	// initial state, and then optional deltas thereafter.

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-subscription.Done():
			return ErrSubscriptionClosed
		case subChanges, ok := <-in:
			if !ok {
				w.logger.Debugf("change channel closed for %q; terminating watcher", w.namespace)
				return nil
			}

			// Allow the possibility of the mapper to drop/filter events.
			changed, err := w.mapper(ctx, w.watchableDB, subChanges)
			if err != nil {
				return errors.Trace(err)
			}
			// If the mapper has dropped all events, we don't need to do
			// anything.
			if len(changed) == 0 {
				continue
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

// InitialNamespaceChanges retrieves the current state of the world from the
// database, as it concerns this watcher.
func InitialNamespaceChanges(selectAll string) NamespaceQuery {
	return func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var keys []string
		err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, selectAll)
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
}

// EmptyInitialNamespaceChanges returns a query that returns no initial changes.
func EmptyInitialNamespaceChanges() NamespaceQuery {
	return func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		return nil, nil
	}
}
