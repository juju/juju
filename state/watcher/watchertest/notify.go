// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	tomb "gopkg.in/tomb.v1"
)

type NotifyWatcher struct {
	tomb tomb.Tomb
	ch   <-chan struct{}
}

func NewNotifyWatcher(ch <-chan struct{}) *NotifyWatcher {
	w := &NotifyWatcher{ch: ch}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
		w.tomb.Kill(tomb.ErrDying)
	}()
	return w
}

func (w *NotifyWatcher) Changes() <-chan struct{} {
	return w.ch
}

func (w *NotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *NotifyWatcher) Kill() {
	w.tomb.Kill(nil)
}

// KillErr can be used to kill the worker with
// an error, to simulate a failing watcher.
func (w *NotifyWatcher) KillErr(err error) {
	w.tomb.Kill(err)
}

func (w *NotifyWatcher) Err() error {
	return w.tomb.Err()
}

func (w *NotifyWatcher) Wait() error {
	return w.tomb.Wait()
}
