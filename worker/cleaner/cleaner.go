// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"fmt"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// Cleaner is responsible for cleaning up the state.
type Cleaner struct {
	tomb tomb.Tomb
	st   *state.State
}

// NewCleaner returns a Cleaner that runs state.Cleanup()
// if the 
func NewCleaner(st *state.State) *Cleaner {
	c := &Cleaner{st: st}
	go func() {
		defer c.tomb.Done()
		c.tomb.Kill(c.loop())
	}()
	return c
}

func (c *Cleaner) String() string {
	return fmt.Sprintf("cleaner")
}

func (c *Cleaner) Kill() {
	c.tomb.Kill(nil)
}

func (c *Cleaner) Stop() error {
	c.tomb.Kill(nil)
	return c.tomb.Wait()
}

func (c *Cleaner) Wait() error {
	return c.tomb.Wait()
}

func (c *Cleaner) loop() error {
	w := c.st.WatchCleanups()
	defer watcher.Stop(w, &c.tomb)

	for {
		select {
		case <-c.tomb.Dying():
			return tomb.ErrDying
		case <-w.Changes():
			if err := c.st.Cleanup(); err != nil {
				return err
			}
		}
	}
	panic("unreachable")
}
