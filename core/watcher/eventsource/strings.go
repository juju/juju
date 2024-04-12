// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
)

// StringsNotifyWatcher wraps a Watcher[[]string] and provides a
// Watcher[struct{}] interface.
type StringsNotifyWatcher struct {
	catacomb catacomb.Catacomb
	out      chan struct{}
	watcher  Watcher[[]string]
}

// NewStringsNotifyWatcher creates a new StringsNotifyWatcher.
func NewStringsNotifyWatcher(watcher Watcher[[]string]) (*StringsNotifyWatcher, error) {
	w := StringsNotifyWatcher{
		watcher: watcher,
		out:     make(chan struct{}),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{watcher},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return &w, nil
}

func (w *StringsNotifyWatcher) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-w.watcher.Changes():
			if !ok {
				return nil
			}

			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.out <- struct{}{}:
			}
		}
	}
}

func (w *StringsNotifyWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *StringsNotifyWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *StringsNotifyWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *StringsNotifyWatcher) Err() error {
	return w.catacomb.Err()
}

func (w *StringsNotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}
