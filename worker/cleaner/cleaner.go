// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.cleaner")

type StateCleaner interface {
	Cleanup() error
	WatchCleanups() (watcher.NotifyWatcher, error)
}

// Cleaner is responsible for cleaning up the state.
type Cleaner struct {
	st StateCleaner
}

// NewCleaner returns a worker.Worker that runs state.Cleanup()
// if the CleanupWatcher signals documents marked for deletion.
func NewCleaner(st StateCleaner) (worker.Worker, error) {
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: &Cleaner{st: st},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (c *Cleaner) SetUp() (watcher.NotifyWatcher, error) {
	return c.st.WatchCleanups()
}

func (c *Cleaner) Handle(_ <-chan struct{}) error {
	if err := c.st.Cleanup(); err != nil {
		logger.Errorf("cannot cleanup state: %v", err)
	}
	// We do not return the err from Cleanup, because we don't want to stop
	// the loop as a failure
	return nil
}

func (c *Cleaner) TearDown() error {
	// Nothing to cleanup, only state is the watcher
	return nil
}
