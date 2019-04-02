// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"time"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/testing"
)

func NewStringsWatcherC(c *gc.C, watcher cache.StringsWatcher) StringsWatcherC {
	return StringsWatcherC{
		C:       c,
		Watcher: watcher,
	}
}

type StringsWatcherC struct {
	*gc.C
	Watcher cache.StringsWatcher
}

// AssertOneChange fails if no change is sent before a long time has passed; or
// if, subsequent to that, any further change is sent before a short time has
// passed.
func (c StringsWatcherC) AssertOneChange(expected []string) {
	select {
	case obtained, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(obtained, jc.SameContents, expected)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

// AssertMaybeCombinedChanges fails if no change is sent before a long time
// has passed; if an empty change is found; if the change isn't part of the
// changes expected.
func (c StringsWatcherC) AssertMaybeCombinedChanges(expected []string) {
	var found bool
	expectedSet := set.NewStrings(expected...)
	for {
		select {
		case obtained, ok := <-c.Watcher.Changes():
			c.Assert(ok, jc.IsTrue)
			c.Logf("expected %v; obtained %v", expectedSet.Values(), obtained)
			// Maybe the expected changes came thru as 1 change.
			if expectedSet.Size() == len(obtained) {
				c.Assert(obtained, jc.SameContents, expectedSet.Values())
				c.Logf("")
				found = true
				break
			}
			// Remove the obtained results from expected, if nothing is removed
			// from expected, fail here, received bad data.
			leftOver := expectedSet.Difference(set.NewStrings(obtained...))
			if expectedSet.Size() == leftOver.Size() {
				c.Fatalf("obtained %v, not contained in expected %v", obtained, expectedSet.Values())
			}
			expectedSet = leftOver
		case <-time.After(testing.LongWait):
			c.Fatalf("watcher did not send change")
		}
		if found {
			break
		}
	}
}

// AssertNoChange fails if it manages to read a value from Changes before a
// short time has passed.
func (c StringsWatcherC) AssertNoChange() {
	select {
	case _, ok := <-c.Watcher.Changes():
		if ok {
			c.Fatalf("watcher sent unexpected change")
		}
		c.Fatalf("watcher changes channel closed")
	case <-time.After(testing.ShortWait):
	}
}

// AssertStops Kills the watcher and asserts (1) that Wait completes without
// error before a long time has passed; and (2) that Changes channel is closed.
func (c StringsWatcherC) AssertStops() {
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
		if ok {
			c.Fatalf("watcher sent unexpected change")
		}
	default:
		c.Fatalf("channel not closed")
	}
}
