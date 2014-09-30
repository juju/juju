// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	stderrors "errors"

	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	"github.com/juju/juju/state/watcher"
)

type dummyWatcher struct {
	err error
}

func (w *dummyWatcher) Stop() error {
	return w.err
}

func (w *dummyWatcher) Err() error {
	return w.err
}

func (s *FastPeriodSuite) TestStop(c *gc.C) {
	t := &tomb.Tomb{}
	watcher.Stop(&dummyWatcher{nil}, t)
	c.Assert(t.Err(), gc.Equals, tomb.ErrStillAlive)

	watcher.Stop(&dummyWatcher{stderrors.New("BLAM")}, t)
	c.Assert(t.Err(), gc.ErrorMatches, "BLAM")
}

func (s *FastPeriodSuite) TestEnsureErr(c *gc.C) {
	err := watcher.EnsureErr(&dummyWatcher{stderrors.New("POW")})
	c.Assert(err, gc.ErrorMatches, "POW")

	err = watcher.EnsureErr(&dummyWatcher{tomb.ErrStillAlive})
	c.Assert(err, gc.ErrorMatches, "expected .* to be stopped: tomb: still alive")

	err = watcher.EnsureErr(&dummyWatcher{nil})
	c.Assert(err, gc.ErrorMatches, "expected an error from .*, got nil")
}
