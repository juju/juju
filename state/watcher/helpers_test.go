// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"errors"

	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/state/watcher"
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

	watcher.Stop(&dummyWatcher{errors.New("BLAM")}, t)
	c.Assert(t.Err(), gc.ErrorMatches, "BLAM")
}

func (s *FastPeriodSuite) TestMustErr(c *gc.C) {
	err := watcher.MustErr(&dummyWatcher{errors.New("POW")})
	c.Assert(err, gc.ErrorMatches, "POW")

	stillAlive := func() { watcher.MustErr(&dummyWatcher{tomb.ErrStillAlive}) }
	c.Assert(stillAlive, gc.PanicMatches, "watcher is still running")

	noErr := func() { watcher.MustErr(&dummyWatcher{nil}) }
	c.Assert(noErr, gc.PanicMatches, "watcher was stopped cleanly")
}
