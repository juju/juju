// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	tomb "gopkg.in/tomb.v2"

	"github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher"
)

type MockStringsWatcher struct {
	tomb tomb.Tomb
	ch   <-chan []string
}

func NewMockStringsWatcher(ch <-chan []string) *MockStringsWatcher {
	w := &MockStringsWatcher{ch: ch}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (w *MockStringsWatcher) Changes() <-chan []string {
	return w.ch
}

func (w *MockStringsWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *MockStringsWatcher) Kill() {
	w.tomb.Kill(nil)
}

// KillErr can be used to kill the worker with
// an error, to simulate a failing watcher.
func (w *MockStringsWatcher) KillErr(err error) {
	w.tomb.Kill(err)
}

func (w *MockStringsWatcher) Err() error {
	return w.tomb.Err()
}

func (w *MockStringsWatcher) Wait() error {
	return w.tomb.Wait()
}

func NewStringsWatcherC(c *tc.C, watcher watcher.StringsWatcher) StringsWatcherC {
	return StringsWatcherC{
		C:       c,
		Watcher: watcher,
	}
}

type StringsWatcherC struct {
	*tc.C
	Watcher watcher.StringsWatcher
}

// AssertOneChange fails if no change is sent before a long time has passed; or
// if, subsequent to that, any further change is sent before a short time has
// passed.
func (c StringsWatcherC) AssertOneChange() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, tc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

// AssertChanges fails if it cannot read a value from Changes despite waiting a
// long time. It logs, but does not check, the received changes; but will fail
// if the Changes chan is closed.
func (c StringsWatcherC) AssertChanges() {
	select {
	case change, ok := <-c.Watcher.Changes():
		c.Logf("received change: %#v", change)
		c.Assert(ok, tc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

// AssertAtLeastOneChange fails if no change is sent before a long time has
// passed.
func (c StringsWatcherC) AssertAtLeastOneChange() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, tc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
}

// AssertNoChange fails if it manages to read a value from Changes before a
// short time has passed.
func (c StringsWatcherC) AssertNoChange() {
	select {
	case change, ok := <-c.Watcher.Changes():
		if !ok {
			c.Fatalf("watcher closed Changes channel")
		} else {
			c.Fatalf("watcher sent unexpected change: %#v", change)
		}
	case <-time.After(testing.ShortWait):
	}
}

// AssertStops Kills the watcher and asserts (1) that Wait completes without
// error before a long time has passed; and (2) that Changes remains open but
// no values are being sent.
func (c StringsWatcherC) AssertStops() {
	c.assertStops(false)
}

// AssertKilled Kills the watcher and asserts that Wait completes without
// error before a long time has passed.
func (c StringsWatcherC) AssertKilled() {
	c.assertStops(true)
}

func (c StringsWatcherC) assertStops(changesClosed bool) {
	c.Watcher.Kill()
	wait := make(chan error)
	go func() {
		wait <- c.Watcher.Wait()
	}()
	select {
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher never stopped")
	case err := <-wait:
		c.Assert(err, tc.ErrorIsNil)
	}

	select {
	case change, ok := <-c.Watcher.Changes():
		if ok || !changesClosed {
			c.Fatalf("watcher sent unexpected change: (%#v, %v)", change, ok)
		}
	default:
	}
}

func (c StringsWatcherC) AssertChange(expect ...string) {
	c.assertChange(false, expect...)
}

func (c StringsWatcherC) AssertChangeInSingleEvent(expect ...string) {
	c.assertChange(true, expect...)
}

// AssertChangeMaybeIncluding verifies that there is a change that may
// contain zero to all of the passed in strings, and no other changes.
func (c StringsWatcherC) AssertChangeMaybeIncluding(expect ...string) {
	maxCount := len(expect)
	actual := c.collectChanges(true, maxCount)

	if maxCount == 0 {
		c.Assert(actual, tc.HasLen, 0)
	} else {
		actualCount := len(actual)
		c.Assert(actualCount <= maxCount, tc.IsTrue, tc.Commentf("expected at most %d, got %d", maxCount, actualCount))
		unexpected := set.NewStrings(actual...).Difference(set.NewStrings(expect...))
		c.Assert(unexpected.Values(), tc.HasLen, 0)
	}
}

// assertChange asserts the given list of changes was reported by
// the watcher, but does not assume there are no following changes.
func (c StringsWatcherC) assertChange(single bool, expect ...string) {
	actual := c.collectChanges(single, len(expect))
	if len(expect) == 0 {
		c.Assert(actual, tc.HasLen, 0)
	} else {
		c.Assert(actual, tc.SameContents, expect)
	}
}

// collectChanges gets up to the max number of changes within the
// testing.LongWait period.
func (c StringsWatcherC) collectChanges(single bool, max int) []string {
	timeout := time.After(testing.LongWait)
	var actual []string
	gotOneChange := false
loop:
	for {
		select {
		case changes, ok := <-c.Watcher.Changes():
			c.Assert(ok, tc.IsTrue)
			gotOneChange = true
			actual = append(actual, changes...)
			if single || len(actual) >= max {
				break loop
			}
		case <-timeout:
			if !gotOneChange {
				c.Fatalf("watcher did not send change")
			}
			break loop
		}
	}
	return actual
}
