// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"context"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
)

// ValueWatcher watches for events associated with a single value
// from a namespace.
// Any time the identified change value has an associated event,
// a notification is emitted.
type ValueWatcher struct {
	*BaseWatcher

	out        chan struct{}
	namespace  string
	changeMask changestream.ChangeType
	filter     func(changestream.ChangeEvent) bool

	mapper Mapper
}

// NewValueWatcher returns a new watcher that receives changes from the input
// base watcher's db/queue when change-log events occur for a specific changeValue
// from the input namespace.
func NewValueWatcher(
	base *BaseWatcher, namespace, changeValue string, changeMask changestream.ChangeType,
) *ValueWatcher {
	return NewValueMapperWatcher(base, namespace, changeValue, changeMask, defaultMapper)
}

// NewValueMapperWatcher returns a new watcher that receives changes from
// the input base watcher's db/queue when mapper accepts the change-log
// events for a specific changeValue from the input namespace.
func NewValueMapperWatcher(
	base *BaseWatcher, namespace, changeValue string, changeMask changestream.ChangeType, mapper Mapper,
) *ValueWatcher {
	w := &ValueWatcher{
		BaseWatcher: base,
		out:         make(chan struct{}),
		namespace:   namespace,
		filter: func(e changestream.ChangeEvent) bool {
			return e.Changed() == changeValue
		},
		changeMask: changeMask,
		mapper:     mapper,
	}

	w.tomb.Go(w.loop)
	return w
}

// NewNamespaceNotifyWatcher returns a new watcher that receives changes from the input
// base watcher's db/queue when changes in the namespace occur.
func NewNamespaceNotifyWatcher(base *BaseWatcher, namespace string, changeMask changestream.ChangeType) *ValueWatcher {
	return NewNamespaceNotifyMapperWatcher(base, namespace, changeMask, defaultMapper)
}

// NewNamespaceNotifyMapperWatcher returns a new watcher that receives changes from
// the input base watcher's db/queue when changes in the namespace occur.
func NewNamespaceNotifyMapperWatcher(
	base *BaseWatcher, namespace string, changeMask changestream.ChangeType, mapper Mapper,
) *ValueWatcher {
	w := &ValueWatcher{
		BaseWatcher: base,
		out:         make(chan struct{}),
		namespace:   namespace,
		filter:      func(changestream.ChangeEvent) bool { return true },
		changeMask:  changeMask,
		mapper:      mapper,
	}

	w.tomb.Go(w.loop)
	return w
}

// Changes returns the channel on which notifications
// are sent when the watched database row changes.
func (w *ValueWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *ValueWatcher) loop() error {
	defer close(w.out)

	opt := changestream.FilteredNamespace(w.namespace, w.changeMask, w.filter)
	subscription, err := w.watchableDB.Subscribe(opt)
	if err != nil {
		return errors.Annotatef(err, "subscribing to namespace %q", w.namespace)
	}
	defer subscription.Unsubscribe()

	// By reassigning the in and out channels, we effectively ticktock between
	// read mode and dispatch mode. This ensures we always dispatch
	// notifications for changes we received before reading more, and every
	// channel read/write is guarded by checks of the tomb and subscription
	// liveness.
	// We begin in dispatch mode in order to send the initial notification.
	in := subscription.Changes()
	out := w.out

	w.drainInitialEvent(in)

	// Cache the context, so we don't have to call it on every iteration.
	ctx := w.tomb.Context(context.Background())

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-subscription.Done():
			return ErrSubscriptionClosed
		case changes, ok := <-in:
			if !ok {
				w.logger.Debugf(context.TODO(), "change channel closed for %q; terminating watcher", w.namespace)
				return nil
			}

			// Allow the possibility of the mapper to drop/filter events.
			changed, err := w.mapper(ctx, w.watchableDB, changes)
			if err != nil {
				return errors.Trace(err)
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

func (w *ValueWatcher) drainInitialEvent(in <-chan []changestream.ChangeEvent) {
	select {
	case _, ok := <-in:
		if !ok {
			w.logger.Debugf(context.TODO(), "change channel closed for %q; terminating watcher", w.namespace)
			return
		}
	default:
	}
}
