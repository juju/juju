// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"errors"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
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

func (s *FastPeriodSuite) TestStop(c *C) {
	t := &tomb.Tomb{}
	watcher.Stop(&dummyWatcher{nil}, t)
	c.Assert(t.Err(), Equals, tomb.ErrStillAlive)

	watcher.Stop(&dummyWatcher{errors.New("BLAM")}, t)
	c.Assert(t.Err(), ErrorMatches, "BLAM")
}

func (s *FastPeriodSuite) TestMustErr(c *C) {
	err := watcher.MustErr(&dummyWatcher{errors.New("POW")})
	c.Assert(err, ErrorMatches, "POW")

	stillAlive := func() { watcher.MustErr(&dummyWatcher{tomb.ErrStillAlive}) }
	c.Assert(stillAlive, PanicMatches, "watcher is still running")

	noErr := func() { watcher.MustErr(&dummyWatcher{nil}) }
	c.Assert(noErr, PanicMatches, "watcher was stopped cleanly")
}
