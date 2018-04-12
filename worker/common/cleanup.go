// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"gopkg.in/juju/worker.v1"
	"sync"
)

// NewCleanupWorker returns a worker that ensures a cleanup function
// is run after the underlying worker is finished.
func NewCleanupWorker(w worker.Worker, cleanup func()) worker.Worker {
	return &CleanupWorker{
		Worker:  w,
		cleanup: cleanup,
	}
}

// CleanupWorker wraps another worker to ensure a func is run when it
// is finished. (Public for manifolds that need access to the
// wrapped worker for output.)
type CleanupWorker struct {
	worker.Worker
	cleanupOnce sync.Once
	cleanup     func()
}

// Wait ensures the cleanup func is run after the worker finishes.
func (w *CleanupWorker) Wait() error {
	err := w.Worker.Wait()
	w.cleanupOnce.Do(w.cleanup)
	return err
}
