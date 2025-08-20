// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"
	"database/sql"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
)

// NamespaceQuery is a function that returns the initial state of a
// namespace watcher.
type NamespaceQuery Query[[]string]

// NamespaceWatcher watches for changes in a namespace.
// Any time events matching the change mask occur in the namespace,
// the values associated with the events are emitted.
type NamespaceWatcher struct {
	*BaseWatcher

	// TODO (manadart 2023-05-24): Consider making this plural (composite key)
	// if/when it is supported by the change log table structure and stream.
	initialQuery NamespaceQuery
	summary      string

	out        chan []string
	filterOpts []changestream.SubscriptionOption
	mapper     Mapper
}

// NewNamespaceWatcher returns a new watcher that filters changes from the input
// base watcher's db/queue. A single filter option is required, though
// additional filter options can be provided.
func NewNamespaceWatcher(
	base *BaseWatcher,
	initialQuery NamespaceQuery,
	summary string,
	filterOption FilterOption, filterOptions ...FilterOption,
) (watcher.StringsWatcher, error) {
	return NewNamespaceMapperWatcher(
		base,
		initialQuery,
		summary,
		defaultMapper,
		filterOption,
		filterOptions...,
	)
}

// NewNamespaceMapperWatcher returns a new watcher that receives changes from
// the input base watcher's db/queue. A single filter option is required, though
// additional filter options can be provided. Filtering of values is done first
// by the filter, and then subsequently by the mapper. Based on the mapper's
// logic a subset of them (or none) may be emitted.
func NewNamespaceMapperWatcher(
	base *BaseWatcher,
	initialQuery NamespaceQuery,
	summary string,
	mapper Mapper,
	filterOption FilterOption, filterOptions ...FilterOption,
) (watcher.StringsWatcher, error) {
	filters := append([]FilterOption{filterOption}, filterOptions...)
	opts := make([]changestream.SubscriptionOption, len(filters))
	for i, opt := range filters {
		if opt == nil {
			return nil, errors.Errorf("nil filter option provided at index %d", i)
		}

		predicate := opt.ChangePredicate()
		if predicate == nil {
			return nil, errors.Errorf("no change predicate provided for filter option %d", i)
		}

		opts[i] = changestream.FilteredNamespace(opt.Namespace(), opt.ChangeMask(), func(e changestream.ChangeEvent) bool {
			return predicate(e.Changed())
		})
	}

	w := &NamespaceWatcher{
		BaseWatcher:  base,
		summary:      summary,
		out:          make(chan []string),
		initialQuery: initialQuery,
		filterOpts:   opts,
		mapper:       mapper,
	}

	w.tomb.Go(w.loop)
	return w, nil
}

// Changes returns the channel on which the keys for
// changed rows are sent to downstream consumers.
func (w *NamespaceWatcher) Changes() <-chan []string {
	return w.out
}

// Report returns a summary of the watcher state.
func (w *NamespaceWatcher) Report() map[string]any {
	return map[string]any{
		"type":    "NamespaceWatcher",
		"summary": w.summary,
	}
}

func (w *NamespaceWatcher) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	defer close(w.out)

	subscription, err := w.watchableDB.Subscribe(w.summary, w.filterOpts...)
	if err != nil {
		return errors.Errorf("subscribing to namespaces: %w", err)
	}
	defer subscription.Kill()

	changes, err := w.initialQuery(ctx, w.watchableDB)
	if err != nil {
		return errors.Errorf("retrieving initial watcher state: %w", err)
	}

	// By reassigning the in and out channels, we effectively ticktock between
	// read mode and dispatch mode. This ensures we always dispatch deltas that
	// we received before reading more, and every channel read/write is guarded
	// by checks of the tomb and subscription liveness.
	// We begin in dispatch mode in order to send the initial state.
	var in <-chan []changestream.ChangeEvent
	out := w.out

	// Note: we don't use the mapper to prevent the initial event. All
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
				w.logger.Debugf(ctx, "change channel closed; terminating watcher")
				return nil
			}

			// Allow the possibility of the mapper to drop/filter events.
			changed, err := w.mapper(ctx, subChanges)
			if err != nil {
				return errors.Capture(err)
			}
			// If the mapper has dropped all events, we don't need to do
			// anything.
			if len(changed) == 0 {
				continue
			}

			// We have changes. Tick over to dispatch mode.
			changes = changed
			in = nil
			out = w.out
		case out <- changes:
			// We have dispatched. Tick over to read mode.
			in = subscription.Changes()
			out = nil
		}
	}
}

func (w *NamespaceWatcher) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}

// InitialNamespaceChanges retrieves the current state of the world from the
// database, as it concerns this watcher.
func InitialNamespaceChanges(selectAll string, args ...any) NamespaceQuery {
	return func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var keys []string
		err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, selectAll, args...)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil
				}
				return errors.Capture(err)
			}
			defer func() { _ = rows.Close() }()

			for i := 0; rows.Next(); i++ {
				var key string
				if err := rows.Scan(&key); err != nil {
					return errors.Capture(err)
				}
				keys = append(keys, key)
			}

			if err := rows.Err(); err != nil {
				return errors.Capture(err)
			}
			return errors.Capture(rows.Close())
		})

		return keys, errors.Capture(err)
	}
}

// EmptyInitialNamespaceChanges returns a query that returns no initial changes.
func EmptyInitialNamespaceChanges() NamespaceQuery {
	return func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		return nil, nil
	}
}
