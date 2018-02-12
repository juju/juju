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
	return &cleanupWorker{
		Worker:  w,
		cleanup: cleanup,
	}
}

type cleanupWorker struct {
	worker.Worker
	cleanupOnce sync.Once
	cleanup     func()
}

func (w *cleanupWorker) Wait() error {
	err := w.Worker.Wait()
	w.cleanupOnce.Do(w.cleanup)
	return err
}
