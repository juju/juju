// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//+build !windows

package reboot_test

import (
	"testing"

	"github.com/juju/juju/worker/common/reboot"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type monitorSuite struct{}

var _ = gc.Suite(&monitorSuite{})

func (s *monitorSuite) TestQueryMonitor(c *gc.C) {
	transientDir := c.MkDir()
	mon := reboot.NewMonitor(transientDir)

	unit, err := names.ParseUnitTag("unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	// Since we pointed the monitor to an empty dir, querying it should
	// return true (reboot detected) and the flag file will be created.
	rebootDetected, err := mon.Query(unit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rebootDetected, jc.IsTrue)

	// Querying the monitor a second time should return false as we
	// already processed the reboot notification
	rebootDetected, err = mon.Query(unit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rebootDetected, jc.IsFalse, gc.Commentf("got unexpected reboot notification"))

	// If we purge the monitor's state for this entity we can query it
	// again and get a reboot detected notification.
	err = mon.PurgeState(unit)
	c.Assert(err, jc.ErrorIsNil)

	rebootDetected, err = mon.Query(unit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rebootDetected, jc.IsTrue)
}

func (s *monitorSuite) TestQueryMonitorForDifferentEntities(c *gc.C) {
	transientDir := c.MkDir()
	mon := reboot.NewMonitor(transientDir)

	unit1, err := names.ParseUnitTag("unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)
	unit2, err := names.ParseUnitTag("unit-mysql-0")
	c.Assert(err, jc.ErrorIsNil)

	// Querying for different entities with no prior monitor state should
	// yield a reboot notification for each entity.
	rebootDetected, err := mon.Query(unit1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rebootDetected, jc.IsTrue)

	rebootDetected, err = mon.Query(unit2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rebootDetected, jc.IsTrue)

	// Querying the monitor a second time should return false as we
	// already processed the reboot notification for both entities
	rebootDetected, err = mon.Query(unit1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rebootDetected, jc.IsFalse, gc.Commentf("got unexpected reboot notification"))
	rebootDetected, err = mon.Query(unit2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rebootDetected, jc.IsFalse, gc.Commentf("got unexpected reboot notification"))
}
func TestAll(t *testing.T) {
	gc.TestingT(t)
}
