// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package minunitsworker_test

import (
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/minunitsworker"
)

var logger = loggo.GetLogger("juju.worker.minunitsworker_test")

type minUnitsWorkerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&minUnitsWorkerSuite{})

func (s *minUnitsWorkerSuite) TestMinUnitsWorker(c *gc.C) {
	mu := minunitsworker.NewMinUnitsWorker(s.State)
	defer func() { c.Assert(worker.Stop(mu), gc.IsNil) }()

	// Set up applications and units for later use.
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Set up minimum units for applications.
	err = wordpress.SetMinUnits(3)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql.SetMinUnits(2)
	c.Assert(err, jc.ErrorIsNil)

	// Remove a unit for an application.
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
