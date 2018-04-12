// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

// period is the amount of time to wait before running cleanups,
// since the last time they were run. It is necessary to run
// cleanups periodically because Cleanup will not return an
// error if a specific cleanup fails, and the watcher will not
// be triggered unless a new cleanup is added.
const period = 30 * time.Second

var logger = loggo.GetLogger("juju.worker.cleaner")

type StateCleaner interface {
	Cleanup() error
	WatchCleanups() (watcher.NotifyWatcher, error)
}

// Cleaner is responsible for cleaning up the state.
type Cleaner struct {
	catacomb catacomb.Catacomb
	st       StateCleaner
	watcher  watcher.NotifyWatcher
	clock    clock.Clock
}

// NewCleaner returns a worker.Worker that runs state.Cleanup()
// periodically, and whenever the CleanupWatcher signals documents
// marked for deletion.
func NewCleaner(st StateCleaner, clock clock.Clock) (worker.Worker, error) {
	watcher, err := st.WatchCleanups()
	if err != nil {
		return nil, errors.Trace(err)
	}
	c := Cleaner{
		st:      st,
		watcher: watcher,
		clock:   clock,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &c.catacomb,
		Work: c.loop,
		Init: []worker.Worker{watcher},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return &c, nil
}

func (c *Cleaner) loop() error {
	timer := c.clock.NewTimer(period)
	defer timer.Stop()
	for {
		select {
		case <-c.catacomb.Dying():
			return c.catacomb.ErrDying()
		case _, ok := <-c.watcher.Changes():
			if !ok {
				return errors.New("change channel closed")
			}
		case <-timer.Chan():
		}
		err := c.st.Cleanup()
		if err != nil {
			// We don't exit if a cleanup fails, we just
			// retry after when the timer fires. This
			// enables us to retry cleanups that fail due
			// to a transient failure, even when there
			// are no new cleanups added.
			logger.Errorf("cannot cleanup state: %v", err)
		}
		timer.Reset(period)
	}
}

// Kill is part of the worker.Worker interface.
func (c *Cleaner) Kill() {
	c.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (c *Cleaner) Wait() error {
	return c.catacomb.Wait()
}
