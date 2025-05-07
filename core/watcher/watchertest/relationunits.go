// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher"
)

// NewRelationUnitsWatcherC returns a RelationUnitsWatcherC that
// checks for aggressive event coalescence.
func NewRelationUnitsWatcherC(c *tc.C, w watcher.RelationUnitsWatcher) RelationUnitsWatcherC {
	return RelationUnitsWatcherC{
		C:                   c,
		Watcher:             w,
		settingsVersions:    make(map[string]int64),
		appSettingsVersions: make(map[string]int64),
	}
}

type RelationUnitsWatcherC struct {
	*tc.C
	Watcher             watcher.RelationUnitsWatcher
	PreAssert           func()
	settingsVersions    map[string]int64
	appSettingsVersions map[string]int64
}

func (c RelationUnitsWatcherC) AssertNoChange() {
	select {
	case actual, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%#v, %v)", actual, ok)
	case <-time.After(testing.ShortWait):
	}
}

// AssertChange asserts the given changes was reported by the watcher,
// but does not assume there are no following changes.
func (c RelationUnitsWatcherC) AssertChange(changed []string, appChanged []string, departed []string) {
	// Get all items in changed in a map for easy lookup.
	changedNames := set.NewStrings(changed...)
	appChangedNames := set.NewStrings(appChanged...)
	timeout := time.After(testing.LongWait)
	select {
	case actual, ok := <-c.Watcher.Changes():
		c.Logf("got change %v", actual)
		c.Assert(ok, jc.IsTrue)
		c.Check(actual.Changed, tc.HasLen, len(changed))
		c.Check(actual.AppChanged, tc.HasLen, len(appChanged))
		// Because the versions can change, we only need to make sure
		// the keys match, not the contents (UnitSettings == txnRevno).
		for k, settings := range actual.Changed {
			c.Check(changedNames.Contains(k), jc.IsTrue)
			oldVer, ok := c.settingsVersions[k]
			if !ok {
				// This is the first time we see this unit, so
				// save the settings version for later.
				c.settingsVersions[k] = settings.Version
			} else {
				// Already seen; make sure the version increased.
				c.Assert(settings.Version, jc.GreaterThan, oldVer,
					tc.Commentf("expected unit settings to increase got %d had %d",
						settings.Version, oldVer))
			}
		}
		for k, version := range actual.AppChanged {
			c.Assert(appChangedNames.Contains(k), jc.IsTrue)
			oldVer, ok := c.appSettingsVersions[k]
			if ok {
				// Make sure if we've seen this setting before, it has been updated
				c.Assert(version, jc.GreaterThan, oldVer,
					tc.Commentf("expected app settings to increase got %d had %d",
						version, oldVer))
			}
			c.appSettingsVersions[k] = version
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
		wait <- c.Watcher.Wait()
	}()
	select {
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher never stopped")
	case err := <-wait:
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case change, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%#v, %v)", change, ok)
	default:
	}
}
