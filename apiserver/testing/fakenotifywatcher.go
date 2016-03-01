// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"launchpad.net/tomb"

	"github.com/juju/juju/state"
)

// FakeNotifyWatcher is an implementation of state.NotifyWatcher which
// is useful in tests.
type FakeNotifyWatcher struct {
	tomb tomb.Tomb
	C    chan struct{}
}

var _ state.NotifyWatcher = (*FakeNotifyWatcher)(nil)

func NewFakeNotifyWatcher() *FakeNotifyWatcher {
	return &FakeNotifyWatcher{
		C: make(chan struct{}, 1),
	}
}

func (w *FakeNotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *FakeNotifyWatcher) Kill() {
	w.tomb.Kill(nil)
	w.tomb.Done()
}

func (w *FakeNotifyWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *FakeNotifyWatcher) Err() error {
	return w.tomb.Err()
}

func (w *FakeNotifyWatcher) Changes() <-chan struct{} {
	return w.C
}
