// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
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

	out         chan struct{}
	namespace   string
	changeValue string
}

// NewValueWatcher returns a new watcher that receives changes from the input
// base watcher's db/queue when change-log events occur for a specific changeValue
// from the input namespace.
func NewValueWatcher(base *BaseWatcher, namespace string, changeValue string) *ValueWatcher {
	w := &ValueWatcher{
		BaseWatcher: base,
		out:         make(chan struct{}),
		namespace:   namespace,
		changeValue: changeValue,
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

	opt := changestream.FilteredNamespace(w.namespace, changestream.All, func(e changestream.ChangeEvent) bool {
		return e.Changed() == w.changeValue
	})
	subscription, err := w.watchableDB.Subscribe(opt)
	if err != nil {
		return errors.Annotatef(err, "subscribing to entity %q in namespace %q", w.changeValue, w.namespace)
	}
	defer subscription.Unsubscribe()

	// By reassigning the in and out channels, we effectively ticktock between
	// read mode and dispatch mode. This ensures we always dispatch
	// notifications for changes we received before reading more, and every
	// channel read/write is guarded by checks of the tomb and subscription
	// liveness.
	// We begin in dispatch mode in order to send the initial notification.
	var in <-chan []changestream.ChangeEvent
	out := w.out

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-subscription.Done():
			return ErrSubscriptionClosed
		case _, ok := <-in:
			if !ok {
				w.logger.Debugf("change channel closed for %q; terminating watcher for %q", w.namespace, w.changeValue)
				return nil
			}

			// We have changes. Tick over to dispatch mode.
			in = nil
			out = w.out
		case out <- struct{}{}:
			// We have dispatched. Tick over to read mode.
			in = subscription.Changes()
			out = nil
		}
	}
}

// Kill (worker.Worker) kills the watcher via its tomb.
func (w *ValueWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait (worker.Worker) waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *ValueWatcher) Wait() error {
	return w.tomb.Wait()
}
