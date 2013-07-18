// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package minuniter_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/minuniter"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type MinuniterSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&MinuniterSuite{})

var _ worker.Worker = (*minuniter.MinUniter)(nil)

func (s *MinuniterSuite) TestMinUniter(c *C) {
	mu := minuniter.NewMinUniter(s.State)
	defer func() { c.Assert(mu.Stop(), IsNil) }()

	// Set up services and units for later use.
	wordpress, err = s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	mysql, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Observe minimum units with a watcher.
	w := s.State.WatchMinUnits()
	defer func() { c.Assert(w.Stop(), IsNil) }()

	// Set up minimum units for services.
	// TODO.

	timeout := time.After(500 * time.Millisecond)
	for {
		s.State.StartSync()
		select {
		case <-time.After(50 * time.Millisecond):
			continue
		case <-timeout:
			c.Fatalf("timed out waiting for cleanup")
		case <-w.Changes():
			needed, err = s.State.NeedsCleanup()
			c.Assert(err, IsNil)
			if needed {
				continue
			}
		}
		break
	}
}
