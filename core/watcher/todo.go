// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import "gopkg.in/tomb.v2"

// TODO returns a watcher for type T that sends an initial change
// with the empty value of type T.
func TODO[T any]() Watcher[T] {
	var empty T
	ch := make(chan T, 1)
	ch <- empty
	w := &todoWatcher[T]{
		ch: ch,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		close(w.ch)
		return tomb.ErrDying
	})
	return w
}

type todoWatcher[T any] struct {
	tomb tomb.Tomb
	ch   chan T
}

func (w *todoWatcher[T]) Kill() {
	w.tomb.Kill(nil)
}

func (w *todoWatcher[T]) Wait() error {
	return w.tomb.Wait()
}

func (w *todoWatcher[T]) Changes() <-chan T {
	return w.ch
}

func (w *todoWatcher[T]) Report() map[string]any {
	return map[string]any{
		"type": "TODOWatcher",
	}
}
