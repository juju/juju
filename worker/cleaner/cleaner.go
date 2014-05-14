// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"github.com/juju/loggo"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker.cleaner")

// Cleaner is responsible for cleaning up the state.
type Cleaner struct {
	st *state.State
}

// NewCleaner returns a worker.Worker that runs state.Cleanup()
// if the CleanupWatcher signals documents marked for deletion.
func NewCleaner(st *state.State) worker.Worker {
	return worker.NewNotifyWorker(&Cleaner{st: st})
}

func (c *Cleaner) SetUp() (watcher.NotifyWatcher, error) {
	return c.st.WatchCleanups(), nil
}

func (c *Cleaner) Handle() error {
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
