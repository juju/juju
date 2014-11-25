// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package minunitsworker_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/minunitsworker"
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
	c.Assert(err, jc.ErrorIsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Set up minimum units for services.
	err = wordpress.SetMinUnits(3)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql.SetMinUnits(2)
	c.Assert(err, jc.ErrorIsNil)

	// Remove a unit for a service.
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	timeout := time.After(coretesting.LongWait)
	for {
		s.State.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			wordpressUnits, err := wordpress.AllUnits()
			c.Assert(err, jc.ErrorIsNil)
			mysqlUnits, err := mysql.AllUnits()
			c.Assert(err, jc.ErrorIsNil)
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
