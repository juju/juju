// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

// TODO returns a watcher for type T that sends an initial change
// with the empty value of type T.
func TODO[T any]() Watcher[T] {
	var empty T
	ch := make(chan T, 1)
	ch <- empty
	w := &todoWatcher[T]{
		ch:   ch,
		done: make(chan struct{}),
	}
	return w
}

type todoWatcher[T any] struct {
	ch   chan T
	done chan struct{}
}

func (w *todoWatcher[T]) Kill() {
	select {
	case <-w.done:
	default:
		close(w.done)
		close(w.ch)
	}
}

func (w *todoWatcher[T]) Wait() error {
	<-w.done
	return nil
}

func (w *todoWatcher[T]) Changes() <-chan T {
	return w.ch
}
