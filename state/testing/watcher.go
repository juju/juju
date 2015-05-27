// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
)

type Stopper interface {
	Stop() error
}

func AssertStop(c *gc.C, stopper Stopper) {
	c.Assert(stopper.Stop(), gc.IsNil)
}

type KillWaiter interface {
	Kill()
	Wait() error
}

func AssertKillAndWait(c *gc.C, killWaiter KillWaiter) {
	killWaiter.Kill()
	c.Assert(killWaiter.Wait(), gc.IsNil)
}

// AssertCanStopWhenSending ensures even when there are changes
// pending to be delivered by the watcher it can still stop
// cleanly. This is necessary to check for deadlocks in case the
// watcher's inner loop is blocked trying to send and its tomb is
// already dying.
func AssertCanStopWhenSending(c *gc.C, stopper Stopper) {
	// Leave some time for the event to be delivered and the watcher
	// to block on sending it.
	<-time.After(testing.ShortWait)
	stopped := make(chan bool)
	// Stop() blocks, so we need to call it in a separate goroutine.
	go func() {
		c.Check(stopper.Stop(), gc.IsNil)
		stopped <- true
	}()
	select {
	case <-time.After(testing.LongWait):
		// NOTE: If this test fails here it means we have a deadlock
		// in the client-side watcher implementation.
		c.Fatalf("watcher did not stop as expected")
	case <-stopped:
	}
}

type NotifyWatcher interface {
	Stop() error
	Changes() <-chan struct{}
}

// NotifyWatcherC embeds a gocheck.C and adds methods to help verify
// the behaviour of any watcher that uses a <-chan struct{}.
type NotifyWatcherC struct {
	*gc.C
	State   SyncStarter
	Watcher NotifyWatcher
}

// SyncStarter is an interface that watcher checkers will use to ensure
// that changes to the watched object have been synchronized. This is
// primarily implemented by state.State.
type SyncStarter interface {
	StartSync()
}

// NewNotifyWatcherC returns a NotifyWatcherC that checks for aggressive
// event coalescence.
func NewNotifyWatcherC(c *gc.C, st SyncStarter, w NotifyWatcher) NotifyWatcherC {
	return NotifyWatcherC{
		C:       c,
		State:   st,
		Watcher: w,
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
	c.State.StartSync()
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
	c.AssertNoChange()
}

func (c NotifyWatcherC) AssertClosed() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsFalse)
	default:
		c.Fatalf("watcher not closed")
	}
}

// StringsWatcherC embeds a gocheck.C and adds methods to help verify
// the behaviour of any watcher that uses a <-chan []string.
type StringsWatcherC struct {
	*gc.C
	State   SyncStarter
	Watcher StringsWatcher
}

// NewStringsWatcherC returns a StringsWatcherC that checks for aggressive
// event coalescence.
func NewStringsWatcherC(c *gc.C, st SyncStarter, w StringsWatcher) StringsWatcherC {
	return StringsWatcherC{
		C:       c,
		State:   st,
		Watcher: w,
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

func (c StringsWatcherC) AssertChanges() {
	c.State.StartSync()
	select {
	case <-c.Watcher.Changes():
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
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
	c.State.StartSync()
	timeout := time.After(testing.LongWait)
	var actual []string
	gotOneChange := false
loop:
	for {
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

func (c StringsWatcherC) AssertClosed() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsFalse)
	default:
		c.Fatalf("watcher not closed")
	}
}

// RelationUnitsWatcherC embeds a gocheck.C and adds methods to help
// verify the behaviour of any watcher that uses a <-chan
// params.RelationUnitsChange.
type RelationUnitsWatcherC struct {
	*gc.C
	State   SyncStarter
	Watcher RelationUnitsWatcher
	// settingsVersions keeps track of the settings version of each
	// changed unit since the last received changes to ensure version
	// always increases.
	settingsVersions map[string]int64
}

// NewRelationUnitsWatcherC returns a RelationUnitsWatcherC that
// checks for aggressive event coalescence.
func NewRelationUnitsWatcherC(c *gc.C, st SyncStarter, w RelationUnitsWatcher) RelationUnitsWatcherC {
	return RelationUnitsWatcherC{
		C:                c,
		State:            st,
		Watcher:          w,
		settingsVersions: make(map[string]int64),
	}
}

type RelationUnitsWatcher interface {
	Stop() error
	Changes() <-chan multiwatcher.RelationUnitsChange
}

func (c RelationUnitsWatcherC) AssertNoChange() {
	c.State.StartSync()
	select {
	case actual, ok := <-c.Watcher.Changes():
		c.Fatalf("watcher sent unexpected change: (%v, %v)", actual, ok)
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
	c.State.StartSync()
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

func (c RelationUnitsWatcherC) AssertClosed() {
	select {
	case _, ok := <-c.Watcher.Changes():
		c.Assert(ok, jc.IsFalse)
	default:
		c.Fatalf("watcher not closed")
	}
}
