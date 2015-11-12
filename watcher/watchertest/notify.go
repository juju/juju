// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
)

func NewNotifyWatcherC(c *gc.C, watcher watcher.NotifyWatcher, preAssert func()) NotifyWatcherC {
	if preAssert == nil {
		preAssert = func() {}
	}
	return NotifyWatcherC{
		C:         c,
		Watcher:   watcher,
		PreAssert: preAssert,
	}
}

type NotifyWatcherC struct {
	*gc.C
	Watcher   watcher.NotifyWatcher
	PreAssert func()
}

func (c NotifyWatcherC) AssertOneChange() {
	c.PreAssert()
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

func (c NotifyWatcherC) AssertNoChange() {
	c.PreAssert()
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
	case <-time.After(testing.ShortWait):
	}
}

func (c NotifyWatcherC) AssertStops() {
	c.Watcher.Kill()
	wait := make(chan error)
	go func() {
		wait <- c.Watcher.Wait()
	}()
	select {
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher never stopped")
	case err := <-wait:
		c.Assert(err, jc.ErrorIsNil)
	}
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
	default:
	}
}
