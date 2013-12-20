// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package minunitsworker_test

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/minunitsworker"
)

var logger = loggo.GetLogger("juju.worker.minunitsworker_test")

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type minUnitsWorkerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&minUnitsWorkerSuite{})

var _ worker.StringsWatchHandler = (*minunitsworker.MinUnitsWorker)(nil)

func (s *minUnitsWorkerSuite) TestMinUnitsWorker(c *gc.C) {
	mu := minunitsworker.NewMinUnitsWorker(s.State)
	defer func() { c.Assert(worker.Stop(mu), gc.IsNil) }()

	// Set up services and units for later use.
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	unit, err := wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, gc.IsNil)

	// Set up minimum units for services.
	err = wordpress.SetMinUnits(3)
	c.Assert(err, gc.IsNil)
	err = mysql.SetMinUnits(2)
	c.Assert(err, gc.IsNil)

	// Remove a unit for a service.
	err = unit.Destroy()
	c.Assert(err, gc.IsNil)

	timeout := time.After(coretesting.LongWait)
	for {
		s.State.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			wordpressUnits, err := wordpress.AllUnits()
			c.Assert(err, gc.IsNil)
			mysqlUnits, err := mysql.AllUnits()
			c.Assert(err, gc.IsNil)
			wordpressCount := len(wordpressUnits)
			mysqlCount := len(mysqlUnits)
			if wordpressCount == 3 && mysqlCount == 2 {
				return
			}
			logger.Infof("wordpress units: %d; mysql units: %d", wordpressCount, mysqlCount)
		case <-timeout:
			c.Fatalf("timed out waiting for minunits events")
		}
	}
}
