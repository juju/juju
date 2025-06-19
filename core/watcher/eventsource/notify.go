// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/internal/errors"
)

// NotifyWatcher watches for events associated with a single value from a
// namespace. Any time the identified change value has an associated event, a
// notification is emitted.
type NotifyWatcher struct {
	*BaseWatcher

	out        chan struct{}
	filterOpts []changestream.SubscriptionOption
	mapper     Mapper
}

// NewNotifyWatcher returns a new watcher that filters changes from the input
// base watcher's db/queue. A single filter option is required, though
// additional filter options can be provided.
func NewNotifyWatcher(
	base *BaseWatcher,
	filterOption FilterOption, filterOptions ...FilterOption,
) (*NotifyWatcher, error) {
	return NewNotifyMapperWatcher(base, defaultMapper, filterOption, filterOptions...)
}

// NewNotifyMapperWatcher returns a new watcher that receives changes from the
// input base watcher's db/queue. A single filter option is required, though
// additional filter options can be provided. Filtering of values is done first
// by the filter, and then subsequently by the mapper. Based on the mapper's
// logic a subset of them (or none) may be emitted.
func NewNotifyMapperWatcher(
	base *BaseWatcher, mapper Mapper,
	filterOption FilterOption, filterOptions ...FilterOption,
) (*NotifyWatcher, error) {
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

	w := &NotifyWatcher{
		BaseWatcher: base,
		out:         make(chan struct{}),
		filterOpts:  opts,
		mapper:      mapper,
	}

	w.tomb.Go(w.loop)
	return w, nil
}

// Changes will emit a struct{} on the channel, which will be the coalesced set
// of changes within a given term.
func (w *NotifyWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *NotifyWatcher) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	defer close(w.out)

	subscription, err := w.watchableDB.Subscribe(w.filterOpts...)
	if err != nil {
		return errors.Errorf("subscribing to namespaces: %w", err)
	}
	defer subscription.Kill()

	// By reassigning the in and out channels, we effectively ticktock between
	// read mode and dispatch mode. This ensures we always dispatch
	// notifications for changes we received before reading more, and every
	// channel read/write is guarded by checks of the tomb and subscription
	// liveness.
	// We begin in dispatch mode in order to send the initial notification.
	in := subscription.Changes()
	out := w.out

	w.drainInitialEvent(ctx, in)

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-subscription.Done():
			return ErrSubscriptionClosed
		case changes, ok := <-in:
			if !ok {
				w.logger.Debugf(ctx, "change channel closed; terminating watcher")
				return nil
			}

			// Allow the possibility of the mapper to drop/filter events.
			changed, err := w.mapper(ctx, changes)
			if err != nil {
				return errors.Capture(err)
			}
			// If the mapper has dropped all events, we don't need to do
			// anything.
			if len(changed) == 0 {
				continue
			}

			// We have changes. Tick over to dispatch mode.
			out = w.out
		case out <- struct{}{}:
			// We have dispatched. Tick over to read mode.
			out = nil
		}
	}
}

func (w *NotifyWatcher) drainInitialEvent(ctx context.Context, in <-chan []changestream.ChangeEvent) {
	select {
	case _, ok := <-in:
		if !ok {
			w.logger.Debugf(ctx, "change channel closed; terminating watcher")
			return
		}
	default:
	}
}

func (w *NotifyWatcher) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}
