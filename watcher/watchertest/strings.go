// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
)

func NewStringsWatcherC(c *gc.C, watcher watcher.StringsWatcher, preAssert func()) StringsWatcherC {
	if preAssert == nil {
		preAssert = func() {}
	}
	return StringsWatcherC{
		C:         c,
		Watcher:   watcher,
		PreAssert: preAssert,
	}
}

type StringsWatcherC struct {
	*gc.C
	Watcher   watcher.StringsWatcher
	PreAssert func()
}

// AssertChanges fails if it cannot read a value from Changes despite waiting a
// long time. It logs, but does not check, the received changes; but will fail
// if the Changes chan is closed.
func (c StringsWatcherC) AssertChanges() {
	c.PreAssert()
	select {
	case change, ok := <-c.Watcher.Changes():
		c.Logf("received change: %#v", change)
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

// AssertNoChange fails if it manages to read a value from Changes before a
// short time has passed.
func (c StringsWatcherC) AssertNoChange() {
	c.PreAssert()
	select {
	case change, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%#v, %v)", change, ok)
	case <-time.After(testing.ShortWait):
	}
}

// AssertStops Kills the watcher and asserts (1) that Wait completes without
// error before a long time has passed; and (2) that Changes remains open but
// no values are being sent.
func (c StringsWatcherC) AssertStops() {
	c.Watcher.Kill()
	wait := make(chan error)
	go func() {
		c.PreAssert()
		wait <- c.Watcher.Wait()
	}()
	select {
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher never stopped")
	case err := <-wait:
		c.Assert(err, jc.ErrorIsNil)
	}

	c.PreAssert()
	select {
	case change, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%#v, %v)", change, ok)
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
		c.Assert(actual, gc.HasLen, 0)
	} else {
		actualCount := len(actual)
		c.Assert(actualCount <= maxCount, jc.IsTrue, gc.Commentf("expected at most %d, got %d", maxCount, actualCount))
		unexpected := set.NewStrings(actual...).Difference(set.NewStrings(expect...))
		c.Assert(unexpected.Values(), gc.HasLen, 0)
	}
}

// assertChange asserts the given list of changes was reported by
// the watcher, but does not assume there are no following changes.
func (c StringsWatcherC) assertChange(single bool, expect ...string) {
	actual := c.collectChanges(single, len(expect))
	if len(expect) == 0 {
		c.Assert(actual, gc.HasLen, 0)
	} else {
		c.Assert(actual, jc.SameContents, expect)
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
		c.PreAssert()
		select {
		case changes, ok := <-c.Watcher.Changes():
			c.Assert(ok, jc.IsTrue)
			gotOneChange = true
			actual = append(actual, changes...)
			if single || len(actual) >= max {
				break loop
			}
		case <-timeout:
			if !gotOneChange {
				c.Fatalf("watcher did not send change")
			}
		}
	}
	return actual
}
