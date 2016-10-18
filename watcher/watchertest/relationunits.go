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

// NewRelationUnitsWatcherC returns a RelationUnitsWatcherC that
// checks for aggressive event coalescence.
func NewRelationUnitsWatcherC(c *gc.C, w watcher.RelationUnitsWatcher, preAssert func()) RelationUnitsWatcherC {
	if preAssert == nil {
		preAssert = func() {}
	}
	return RelationUnitsWatcherC{
		C:                c,
		PreAssert:        preAssert,
		Watcher:          w,
		settingsVersions: make(map[string]int64),
	}
}

type RelationUnitsWatcherC struct {
	*gc.C
	Watcher          watcher.RelationUnitsWatcher
	PreAssert        func()
	settingsVersions map[string]int64
}

func (c RelationUnitsWatcherC) AssertNoChange() {
	c.PreAssert()
	select {
	case actual, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%#v, %v)", actual, ok)
	case <-time.After(testing.ShortWait):
	}
}

// AssertChange asserts the given changes was reported by the watcher,
// but does not assume there are no following changes.
func (c RelationUnitsWatcherC) AssertChange(changed []string, departed []string) {
	// Get all items in changed in a map for easy lookup.
	changedNames := make(map[string]bool)
	for _, name := range changed {
		changedNames[name] = true
	}
	c.PreAssert()
	timeout := time.After(testing.LongWait)
	select {
	case actual, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(actual.Changed, gc.HasLen, len(changed))
		// Because the versions can change, we only need to make sure
		// the keys match, not the contents (UnitSettings == txnRevno).
		for k, settings := range actual.Changed {
			_, ok := changedNames[k]
			c.Assert(ok, jc.IsTrue)
			oldVer, ok := c.settingsVersions[k]
			if !ok {
				// This is the first time we see this unit, so
				// save the settings version for later.
				c.settingsVersions[k] = settings.Version
			} else {
				// Already seen; make sure the version increased.
				if settings.Version <= oldVer {
					c.Fatalf("expected unit settings version > %d (got %d)", oldVer, settings.Version)
				}
			}
		}
		c.Assert(actual.Departed, jc.SameContents, departed)
	case <-timeout:
		c.Fatalf("watcher did not send change")
	}
}

// AssertStops Kills the watcher and asserts (1) that Wait completes without
// error before a long time has passed; and (2) that Changes remains open but
// no values are being sent.
func (c RelationUnitsWatcherC) AssertStops() {
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
