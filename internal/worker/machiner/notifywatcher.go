// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/core/watcher"
)

// mergedNotifyWatcher forwards machine watcher changes and address watcher
// changes into a single notify stream. The initial event from the address
// watcher is suppressed so the existing machine watcher startup behavior
// remains unchanged.
type mergedNotifyWatcher struct {
	catacomb catacomb.Catacomb
	changes  chan struct{}

	primary   watcher.NotifyWatcher
	secondary watcher.NotifyWatcher
}

func newMergedNotifyWatcher(
	primary watcher.NotifyWatcher, secondary watcher.NotifyWatcher,
) (watcher.NotifyWatcher, error) {
	w := &mergedNotifyWatcher{
		changes:   make(chan struct{}, 1),
		primary:   primary,
		secondary: secondary,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "machiner-merged-notify-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{primary, secondary},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *mergedNotifyWatcher) loop() error {
	skipSecondaryInitial := true

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case _, ok := <-w.primary.Changes():
			if !ok {
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				default:
					return errors.New("machine change channel closed")
				}
			}
			w.notify()

		case _, ok := <-w.secondary.Changes():
			if !ok {
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				default:
					return errors.New("address change channel closed")
				}
			}
			if skipSecondaryInitial {
				skipSecondaryInitial = false
				continue
			}
			w.notify()
		}
	}
}

func (w *mergedNotifyWatcher) notify() {
	select {
	case <-w.catacomb.Dying():
	case w.changes <- struct{}{}:
	}
}

func (w *mergedNotifyWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *mergedNotifyWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *mergedNotifyWatcher) Changes() <-chan struct{} {
	return w.changes
}
