// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	"fmt"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testing"
)

// WatcherAssert is a function that asserts the changes received by a watcher.
type WatcherAssert[T any] func(c *tc.C, changes []T) bool

// SliceAssert returns a WatcherAssert that checks that the watcher has
// received at least the given changes.
func SliceAssert[T any](expect ...T) WatcherAssert[T] {
	return func(c *tc.C, changes []T) bool {
		if len(changes) >= len(expect) {
			c.Assert(changes, tc.SameContents, expect)
			return true
		}
		return false
	}
}

// StringSliceAssert returns a WatcherAssert that checks that the watcher has
// received at least the given []string changes. The changes are
// concatenated before the assertion, order doesn't matter during assertion.
func StringSliceAssert[T string](expect ...T) WatcherAssert[[]T] {
	return func(c *tc.C, changes [][]T) bool {
		var received []T
		for _, change := range changes {
			received = append(received, change...)
		}
		if len(received) >= len(expect) {
			c.Assert(received, tc.SameContents, expect)
			return true
		}
		return false
	}
}

// TimedSliceAssert returns a WatcherAssert that checks that the watcher has
// received at least the given []T changes. The changes are concatenated before
// the assertion, and the time fields are checked to be within a certain
// tolerance of the expected values. The order of the changes does not matter
// during the assertion.
func TimedSliceAssert[T any](timeFieldNames ...string) func(expect ...T) WatcherAssert[[]T] {
	mc := tc.NewMultiChecker()
	for _, field := range timeFieldNames {
		mc.AddExpr(fmt.Sprintf(`_[_].%s`, field), tc.Almost, tc.ExpectedValue)
	}
	return func(expect ...T) WatcherAssert[[]T] {
		return func(c *tc.C, changes [][]T) bool {
			var received []T
			for _, change := range changes {
				received = append(received, change...)
			}
			if len(received) >= len(expect) {
				c.Assert(received, tc.UnorderedMatch[[]T](mc), expect)
				return true
			}
			return false
		}
	}
}

// WatcherC embeds a gocheck.C and adds methods to help
// verify the behaviour of generic watchers.
type WatcherC[T any] struct {
	c       *tc.C
	Watcher watcher.Watcher[T]
}

// NewWatcherC() returns a WatcherC[T].
func NewWatcherC[T any](c *tc.C, w watcher.Watcher[T]) WatcherC[T] {
	return WatcherC[T]{
		c:       c,
		Watcher: w,
	}
}

// AssertNoChange verifies that no changes are received from the watcher during
// a testing.ShortWait time.
func (w *WatcherC[T]) AssertNoChange() {
	select {
	case actual, ok := <-w.Watcher.Changes():
		w.c.Fatalf("watcher sent unexpected change: (%v, %v)", actual, ok)
	case <-time.After(testing.ShortWait):
	}
}

// AssertChange asserts that the watcher sends at least one change
// before the test times out.
func (w *WatcherC[T]) AssertChange() {
	select {
	case _, ok := <-w.Watcher.Changes():
		w.c.Assert(ok, tc.Equals, true)
	case <-w.c.Context().Done():
		w.c.Fatalf("watcher did not send change")
	}
}

// AssertNChanges fails if it does not receive n changes before a long time has passed.
func (w WatcherC[T]) AssertNChanges(n int) {
	if n <= 1 {
		w.c.Fatalf("n must be greater than 1")
	}
	received := 0
	for {
		select {
		case _, ok := <-w.Watcher.Changes():
			w.c.Assert(ok, tc.IsTrue)
			received++

			if received < n {
				continue
			}
			// Ensure we have no more changes.
			w.AssertNoChange()
			return
		case <-w.c.Context().Done():
			if received == 0 {
				w.c.Fatalf("watcher did not send any changes")
			} else {
				w.c.Fatalf("watcher received %d changes, expected %d", received, n)
			}
		}
	}
}

// CheckInitial asserts that the watcher sends the expected initial changes. The
// assertion function is called only once and returns true, or the test
// times out.
func (w *WatcherC[T]) CheckInitial(assertion WatcherAssert[T]) {
	select {
	case changes, ok := <-w.Watcher.Changes():
		w.c.Logf("WatcherC Watcher.Changes() => %# v", changes)
		if !ok {
			wait := make(chan error)
			go func() {
				wait <- w.Watcher.Wait()
			}()
			select {
			case <-w.c.Context().Done():
				w.c.Fatalf("watcher never stopped")
			case err := <-wait:
				w.c.Fatalf("watcher killed with err: %q", err.Error())
			}
		}

		if assertion(w.c, []T{changes}) {
			return
		}
	case <-w.c.Context().Done():
		w.c.Fatalf("watcher did not send initial change")
	}
}

// Check asserts that the watcher sends the expected changes. The assertion
// function is called repeatedly until it returns true, or the test times out.
func (w *WatcherC[T]) Check(assertion WatcherAssert[T]) {
	var received []T
	for a := testing.LongAttempt.Start(); a.Next(); {
		select {
		case actual, ok := <-w.Watcher.Changes():
			w.c.Logf("WatcherC Watcher.Changes() => %# v", actual)
			if !ok {
				wait := make(chan error)
				go func() {
					wait <- w.Watcher.Wait()
				}()
				select {
				case <-w.c.Context().Done():
					w.c.Fatalf("watcher never stopped")
				case err := <-wait:
					w.c.Fatalf("watcher killed with err: %q", err.Error())
				}
			}

			received = append(received, actual)
			w.c.Logf("received %+v", received)
			if assertion(w.c, received) {
				return
			}
		case <-w.c.Context().Done():
			if len(received) == 0 {
				w.c.Fatalf("watcher did not send change")
			} else {
				w.c.Fatalf("watcher did not send expected changes")
			}
		}
	}
}

// AssertKilled Kills the watcher and asserts that Wait completes without
// error before a long time has passed.
func (w *WatcherC[T]) AssertKilled() {
	w.Watcher.Kill()

	wait := make(chan error)
	go func() {
		wait <- w.Watcher.Wait()
	}()
	select {
	case <-w.c.Context().Done():
		w.c.Fatalf("watcher never stopped")
	case err := <-wait:
		w.c.Assert(err, tc.ErrorIsNil)
	}

	select {
	case _, ok := <-w.Watcher.Changes():
		if ok {
			w.c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		}
	default:
	}
}

// CleanKill calls CheckKill with the supplied arguments, and Checks that the
// returned error is nil. It's particularly suitable for deferring:
//
//	someWatcher, err := some.NewWatcher()
//	c.Assert(err, tc.ErrorIsNil)
//	watchertest.CleanKill(c, someWatcher)
//
// ...in the large number (majority?) of situations where a worker is expected
// to run successfully; and it doesn't Assert, and is therefore suitable for use
// from any goroutine.
func CleanKill[T any](c *tc.C, w watcher.Watcher[T]) {
	workertest.CleanKill(c, w)
	_, ok := <-w.Changes()
	c.Assert(ok, tc.IsFalse)
}

// DirtyKill calls CheckKill with the supplied arguments, and logs the returned
// error. It's particularly suitable for deferring:
//
//	someWatcher, err := some.NewWatcher()
//	c.Assert(err, tc.ErrorIsNil)
//	defer watchertest.DirtyKill(c, someWatcher)
//
// ...in the cases where we expect a worker to fail, but aren't specifically
// testing that failure; and it doesn't Assert, and is therefore suitable for
// use from any goroutine.
func DirtyKill[T any](c *tc.C, w watcher.Watcher[T]) {
	workertest.DirtyKill(c, w)
	_, ok := <-w.Changes()
	if !ok {
		c.Logf("ignoring failed to close for watcher")
	}
}
