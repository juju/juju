// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
)

type waitForEnvironSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&waitForEnvironSuite{})

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

func (s *waitForEnvironSuite) TestStop(c *gc.C) {
	w := s.State.WatchForEnvironConfigChanges()
	defer stopWatcher(c, w)
	stop := make(chan struct{})
	done := make(chan error)
	go func() {
		env, err := worker.WaitForEnviron(w, s.State, stop)
		c.Check(env, gc.IsNil)
		done <- err
	}()
	close(stop)
	c.Assert(<-done, gc.Equals, tomb.ErrDying)
}

func stopWatcher(c *gc.C, w state.NotifyWatcher) {
	err := w.Stop()
	c.Check(err, gc.IsNil)
}
