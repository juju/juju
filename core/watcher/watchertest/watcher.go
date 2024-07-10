// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testing"
)

// CleanKill calls CheckKill with the supplied arguments, and Checks that the
// returned error is nil. It's particularly suitable for deferring:
//
//	someWatcher, err := some.NewWatcher()
//	c.Assert(err, jc.ErrorIsNil)
//	watchertest.CleanKill(c, someWatcher)
//
// ...in the large number (majority?) of situations where a worker is expected
// to run successfully; and it doesn't Assert, and is therefore suitable for use
// from any goroutine.
func CleanKill[T any](c *gc.C, w watcher.Watcher[T]) {
	workertest.CleanKill(c, w)
	_, ok := <-w.Changes()
	c.Assert(ok, jc.IsFalse)
}

// DirtyKill calls CheckKill with the supplied arguments, and logs the returned
// error. It's particularly suitable for deferring:
//
//	someWatcher, err := some.NewWatcher()
//	c.Assert(err, jc.ErrorIsNil)
//	defer watchertest.DirtyKill(c, someWatcher)
//
// ...in the cases where we expect a worker to fail, but aren't specifically
// testing that failure; and it doesn't Assert, and is therefore suitable for
// use from any goroutine.
func DirtyKill[T any](c *gc.C, w watcher.Watcher[T]) {
	workertest.DirtyKill(c, w)
	_, ok := <-w.Changes()
	if !ok {
		c.Logf("ignoring failed to close for watcher")
	}
}

// WatcherC embeds a gocheck.C and adds methods to help
// verify the behaviour of generic watchers.
type WatcherC[T any] struct {
	C       *gc.C
	Watcher watcher.Watcher[T]
}

// NewWatcherC() returns a WatcherC[T].
func NewWatcherC[T any](c *gc.C, w watcher.Watcher[T]) WatcherC[T] {
	return WatcherC[T]{
		C:       c,
		Watcher: w,
	}
}

// AssertNoChange verifies that no changes are received from the watcher during
// a testing.ShortWait time.
func (w *WatcherC[T]) AssertNoChange() {
	select {
	case actual, ok := <-w.Watcher.Changes():
		w.C.Fatalf("watcher sent unexpected change: (%v, %v)", actual, ok)
	case <-time.After(testing.ShortWait):
	}
}

// AssertChange asserts the given changes was reported by the watcher,
// but does not assume there are no following changes.
func (w *WatcherC[T]) AssertChange(assertion func(c *gc.C, received []T) bool) {
	var received []T
	timeout := time.After(testing.LongWait)
	for a := testing.LongAttempt.Start(); a.Next(); {
		select {
		case actual, ok := <-w.Watcher.Changes():
			w.C.Logf("WatcherC Watcher.Changes() => %# v", actual)
			w.C.Assert(ok, jc.IsTrue)
			received = append(received, actual)
			w.C.Logf("received %+v", received)
			if assertion(w.C, received) {
				return
			}
		case <-timeout:
			w.C.Fatalf("watcher did not send change")
		}
	}
}
