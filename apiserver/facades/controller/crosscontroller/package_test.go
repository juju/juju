// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"gopkg.in/tomb.v2"
)

type mockNotifyWatcher struct {
	tomb    tomb.Tomb
	changes chan struct{}
}

func newMockNotifyWatcher() *mockNotifyWatcher {
	w := &mockNotifyWatcher{changes: make(chan struct{}, 42)}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
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

func (w *mockNotifyWatcher) Changes() <-chan struct{} {
	return w.changes
}
