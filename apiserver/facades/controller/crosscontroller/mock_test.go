// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller_test

import "gopkg.in/tomb.v1"

type mockNotifyWatcher struct {
	tomb    tomb.Tomb
	changes chan struct{}
}

func newMockNotifyWatcher() *mockNotifyWatcher {
	w := &mockNotifyWatcher{changes: make(chan struct{}, 1)}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w
}

func (w *mockNotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *mockNotifyWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *mockNotifyWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *mockNotifyWatcher) Err() error {
	return w.tomb.Err()
}

func (w *mockNotifyWatcher) Changes() <-chan struct{} {
	return w.changes
}
