// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"

	"github.com/juju/juju/state"
)

// FakeNotifyWatcher is an implementation of state.NotifyWatcher which
// is useful in tests.
type FakeNotifyWatcher struct {
	worker.Worker
	C chan struct{}
}

var _ state.NotifyWatcher = (*FakeNotifyWatcher)(nil)

func NewFakeNotifyWatcher() *FakeNotifyWatcher {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return &FakeNotifyWatcher{
		Worker: workertest.NewErrorWorker(nil),
		C:      ch,
	}
}

// Stop is part of the state.NotifyWatcher interface.
func (w *FakeNotifyWatcher) Stop() error {
	return worker.Stop(w)
}

// Err is part of the state.NotifyWatcher interface.
func (w *FakeNotifyWatcher) Err() error {
	// this is silly, but it's what it always returned anyway
	return nil
}

// Changes is part of the state.NotifyWatcher interface.
func (w *FakeNotifyWatcher) Changes() <-chan struct{} {
	return w.C
}
