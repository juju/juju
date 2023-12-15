// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"sync"

	"github.com/juju/juju/core/watcher"
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

func (w *mockWatcher) Stopped() bool {
	select {
	case <-w.stopped:
		return true
	default:
		return false
	}
}

func (w *mockWatcher) Wait() error {
	<-w.stopped
	return nil
}

func newMockRelationUnitsWatcher(changes chan watcher.RelationUnitsChange) *mockRelationUnitsWatcher {
	return &mockRelationUnitsWatcher{
		mockWatcher: newMockWatcher(),
		changes:     changes,
	}
}

type mockRelationUnitsWatcher struct {
	*mockWatcher
	changes chan watcher.RelationUnitsChange
}

func (w *mockRelationUnitsWatcher) Changes() watcher.RelationUnitsChannel {
	return w.changes
}
