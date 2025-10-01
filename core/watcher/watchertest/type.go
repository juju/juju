// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import "gopkg.in/tomb.v2"

type MockWatcher[T any] struct {
	tomb tomb.Tomb
	ch   <-chan []T
}

func NewMockWatcher[T any](ch <-chan []T) *MockWatcher[T] {
	w := &MockWatcher[T]{ch: ch}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (w *MockWatcher[T]) Changes() <-chan []T {
	return w.ch
}

func (w *MockWatcher[T]) Kill() {
	w.tomb.Kill(nil)
}

func (w *MockWatcher[T]) Wait() error {
	return w.tomb.Wait()
}
