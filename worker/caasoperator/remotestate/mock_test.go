// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	"sync"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/watcher"
)

func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		stopped: make(chan struct{}),
	}
}

type mockWatcher struct {
	mu      sync.Mutex
	stopped chan struct{}
}

func (w *mockWatcher) Kill() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.Stopped() {
		close(w.stopped)
	}
}

func (w *mockWatcher) Wait() error {
	<-w.stopped
	return nil
}

func (w *mockWatcher) Stopped() bool {
	select {
	case <-w.stopped:
		return true
	default:
		return false
	}
}

func newMockNotifyWatcher() *mockNotifyWatcher {
	return &mockNotifyWatcher{
		mockWatcher: newMockWatcher(),
		changes:     make(chan struct{}, 1),
	}
}

type mockNotifyWatcher struct {
	*mockWatcher
	changes chan struct{}
}

func (w *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

type mockApplicationWatcher struct {
	watcher *mockNotifyWatcher
}

func (s *mockApplicationWatcher) Watch(application string) (watcher.NotifyWatcher, error) {
	if application != "gitlab" {
		return nil, errors.NotFoundf(application)
	}
	return s.watcher, nil
}

type mockCharmGetter struct {
	curl    *charm.URL
	force   bool
	sha256  string
	version int
}

func (m *mockCharmGetter) Charm(application string) (_ *charm.URL, force bool, sha256 string, vers int, _ error) {
	return m.curl, m.force, m.sha256, m.version, nil
}
