// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
)

// KeyWatcher watches for changes to single database table row.
// Any time the identified row changes, a notification is emitted.
type KeyWatcher struct {
	*BaseWatcher

	out       chan struct{}
	tableName string
	keyValue  string
}

// NewKeyWatcher returns a new watcher that receives changes from the input
// base watcher's db/queue when a specific database table row changes.
func NewKeyWatcher(base *BaseWatcher, tableName string, keyValue string) *KeyWatcher {
	w := &KeyWatcher{
		BaseWatcher: base,
		out:         make(chan struct{}),
		tableName:   tableName,
		keyValue:    keyValue,
	}

	w.tomb.Go(w.loop)
	return w
}

// Changes returns the channel on which notifications
// are sent when the watched database row changes.
func (w *KeyWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *KeyWatcher) loop() error {
	defer close(w.out)

	opt := changestream.FilteredNamespace(w.tableName, changestream.All, func(e changestream.ChangeEvent) bool {
		return e.ChangedUUID() == w.keyValue
	})
	subscription, err := w.eventSource.Subscribe(opt)
	if err != nil {
		return errors.Annotatef(err, "subscribing to entity %q in namespace %q", w.keyValue, w.tableName)
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
				w.logger.Debugf("change channel closed for %q; terminating watcher for %q", w.tableName, w.keyValue)
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
func (w *KeyWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait (worker.Worker) waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *KeyWatcher) Wait() error {
	return w.tomb.Wait()
}
