// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher"
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
