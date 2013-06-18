// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"sort"
	"time"
)

var (
	longTime  = 500 * time.Millisecond
	shortTime = 50 * time.Millisecond
)

type Stopper interface {
	Stop() error
}

func AssertStop(c *C, stopper Stopper) {
	c.Assert(stopper.Stop(), IsNil)
}

type NotifyWatcher interface {
	Changes() <-chan struct{}
}

// NotifyWatcherC makes it simple to verify the behaviour of any watcher
// that uses a <-chan struct{} to communicate changes.
type NotifyWatcherC struct {
	*C
	state   *state.State
	watcher NotifyWatcher
}

func (c NotifyWatcherC) AssertNoChange() {
	c.state.StartSync()
	select {
	case _, ok := <-c.watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
	case <-time.After(shortTime):
	}
}

func (c NotifyWatcherC) AssertOneChange() {
	c.state.Sync()
	select {
	case _, ok := <-c.watcher.Changes():
		c.Assert(ok, Equals, true)
	case <-time.After(longTime):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

func (c NotifyWatcherC) AssertClosed() {
	select {
	case _, ok := <-c.watcher.Changes():
		c.Assert(ok, Equals, false)
	default:
		c.Fatalf("watcher not closed")
	}
}

type StringsWatcherC struct {
	*C
	state   *state.State
	watcher StringsWatcher
}

type StringsWatcher interface {
	Stop() error
	Changes() <-chan []string
}

func (c StringsWatcherC) AssertNoChange() {
	c.state.StartSync()
	select {
	case actual, ok := <-c.watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%v, %v)", actual, ok)
	case <-time.After(shortTime):
	}
}

func (c StringsWatcherC) AssertOneChange(expect ...string) {
	c.state.Sync()
	select {
	case actual, ok := <-c.watcher.Changes():
		c.Assert(ok, Equals, true)
		if len(expect) == 0 {
			c.Assert(actual, HasLen, 0)
		} else {
			sort.Strings(expect)
			sort.Strings(actual)
			c.Assert(expect, DeepEquals, actual)
		}
	case <-time.After(longTime):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

func (c StringsWatcherC) AssertClosed() {
	select {
	case _, ok := <-c.watcher.Changes():
		c.Assert(ok, Equals, false)
	default:
		c.Fatalf("watcher not closed")
	}
}

type IntsWatcherC struct {
	*C
	state   *state.State
	watcher IntsWatcher
}

type IntsWatcher interface {
	Stop() error
	Changes() <-chan []int
}

func (c IntsWatcherC) AssertNoChange() {
	c.state.StartSync()
	select {
	case actual, ok := <-c.watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%v, %v)", actual, ok)
	case <-time.After(shortTime):
	}
}

func (c IntsWatcherC) AssertOneChange(expect ...int) {
	c.state.Sync()
	select {
	case actual, ok := <-c.watcher.Changes():
		c.Assert(ok, Equals, true)
		if len(expect) == 0 {
			c.Assert(actual, HasLen, 0)
		} else {
			sort.Ints(expect)
			sort.Ints(actual)
			c.Assert(expect, DeepEquals, actual)
		}
	case <-time.After(longTime):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

func (c IntsWatcherC) AssertClosed() {
	select {
	case _, ok := <-c.watcher.Changes():
		c.Assert(ok, Equals, false)
	default:
		c.Fatalf("watcher not closed")
	}
}
