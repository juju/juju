// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"sort"
	"time"
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

// NotifyWatcherC embeds a gocheck.C and adds methods to help verify
// the behaviour of any watcher that uses a <-chan struct{}.
type NotifyWatcherC struct {
	*C
	State    *state.State
	Watcher  NotifyWatcher
	FullSync bool
}

// NewNotifyWatcherC returns a NotifyWatcherC that checks for aggressive
// event coalescence.
func NewNotifyWatcherC(c *C, st *state.State, w NotifyWatcher) NotifyWatcherC {
	return NotifyWatcherC{
		C:       c,
		State:   st,
		Watcher: w,
	}
}

// NewLaxNotifyWatcherC returns a NotifyWatcherC that runs a full watcher
// sync before reading from the watcher's Changes channel, and hence cannot
// verify real-world coalescence behaviour.
func NewLaxNotifyWatcherC(c *C, st *state.State, w NotifyWatcher) NotifyWatcherC {
	return NotifyWatcherC{
		C:        c,
		State:    st,
		Watcher:  w,
		FullSync: true,
	}
}

func (c NotifyWatcherC) AssertNoChange() {
	c.State.StartSync()
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
	case <-time.After(testing.ShortWait):
	}
}

func (c NotifyWatcherC) AssertOneChange() {
	if c.FullSync {
		c.State.Sync()
	} else {
		c.State.StartSync()
	}
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, Equals, true)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

func (c NotifyWatcherC) AssertClosed() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, Equals, false)
	default:
		c.Fatalf("watcher not closed")
	}
}

// StringsWatcherC embeds a gocheck.C and adds methods to help verify
// the behaviour of any watcher that uses a <-chan []string.
type StringsWatcherC struct {
	*C
	State    *state.State
	Watcher  StringsWatcher
	FullSync bool
}

// NewStringsWatcherC returns a StringsWatcherC that checks for aggressive
// event coalescence.
func NewStringsWatcherC(c *C, st *state.State, w StringsWatcher) StringsWatcherC {
	return StringsWatcherC{
		C:       c,
		State:   st,
		Watcher: w,
	}
}

// NewLaxStringsWatcherC returns a StringsWatcherC that runs a full watcher
// sync before reading from the watcher's Changes channel, and hence cannot
// verify real-world coalescence behaviour.
func NewLaxStringsWatcherC(c *C, st *state.State, w StringsWatcher) StringsWatcherC {
	return StringsWatcherC{
		C:        c,
		State:    st,
		Watcher:  w,
		FullSync: true,
	}
}

type StringsWatcher interface {
	Stop() error
	Changes() <-chan []string
}

func (c StringsWatcherC) AssertNoChange() {
	c.State.StartSync()
	select {
	case actual, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%v, %v)", actual, ok)
	case <-time.After(testing.ShortWait):
	}
}

// AssertChange asserts the given list of changes was reported by
// the watcher, but does not assume there are no following changes.
func (c StringsWatcherC) AssertChange(expect ...string) {
	if c.FullSync {
		c.State.Sync()
	} else {
		c.State.StartSync()
	}
	select {
	case actual, ok := <-c.Watcher.Changes():
		c.Assert(ok, Equals, true)
		if len(expect) == 0 {
			c.Assert(actual, HasLen, 0)
		} else {
			sort.Strings(expect)
			sort.Strings(actual)
			c.Assert(actual, DeepEquals, expect)
		}
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
}

// AssertOneChange asserts the given list of changes was reported by
// the watcher and there are no more changes after that.
func (c StringsWatcherC) AssertOneChange(expect ...string) {
	c.AssertChange(expect...)
	c.AssertNoChange()
}

func (c StringsWatcherC) AssertClosed() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, Equals, false)
	default:
		c.Fatalf("watcher not closed")
	}
}
