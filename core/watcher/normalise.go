// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
)

// Normalise takes any watcher and normalises it down to a NotifyWatcher.
// This is useful in legacy code that expects a NotifyWatcher.
func Normalise[T any](source Watcher[T]) (NotifyWatcher, error) {
	if w, ok := source.(NotifyWatcher); ok {
		// If we are already a NotifyWatcher, we can return the source watcher.
		return w, nil
	}

	ch := make(chan struct{})
	w := &normaliseWatcher{
		ch: ch,
	}
	loop := func() error {
		defer close(ch)
		for {
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()

			case _, ok := <-source.Changes():
				if !ok {
					select {
					case <-w.catacomb.Dying():
						return w.catacomb.ErrDying()
					default:
						return nil
					}
				}
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				case ch <- struct{}{}:
				}
			}
		}
	}

	err := catacomb.Invoke(catacomb.Plan{
		Name: "normalise-watcher",
		Site: &w.catacomb,
		Work: loop,
		Init: []worker.Worker{source},
	})
	if err != nil {
		return nil, err
	}
	return w, nil
}

type normaliseWatcher struct {
	catacomb catacomb.Catacomb
	ch       chan struct{}
}

func (w *normaliseWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *normaliseWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *normaliseWatcher) Changes() <-chan struct{} {
	return w.ch
}
