// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package minunitsworker_test

import (
	stdtesting "testing"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/minunitsworker"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type MinuniterSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&MinuniterSuite{})

var _ worker.Worker = (*minunitsworker.MinUnitsWorker)(nil)

func (s *MinuniterSuite) TestMinUnitsWorker(c *C) {
	mu := minunitsworker.NewMinUnitsWorker(s.State)
	defer func() { c.Assert(mu.Stop(), IsNil) }()

	// Set up services and units for later use.
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Observe minimum units with a watcher.
	w := s.State.WatchMinUnits()
	defer func() { c.Assert(w.Stop(), IsNil) }()

	// Set up minimum units for services.
	err = wordpress.SetMinUnits(3)
	c.Assert(err, IsNil)
	err = mysql.SetMinUnits(2)
	c.Assert(err, IsNil)

	// Remove a unit for a service.
	err = unit.Destroy()
	c.Assert(err, IsNil)

	timeout := time.After(500 * time.Millisecond)
	for {
		s.State.StartSync()
		select {
		case <-time.After(50 * time.Millisecond):
			wordpressUnits, err := wordpress.AllUnits()
			c.Assert(err, IsNil)
			mysqlUnits, err := mysql.AllUnits()
			c.Assert(err, IsNil)
			if len(wordpressUnits) == 3 && len(mysqlUnits) == 2 {
				return
			}
		case <-timeout:
			c.Fatalf("timed out waiting for minunits events")
		}
	}
}
