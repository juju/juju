// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/worker/common/reboot"
)

type monitorSuite struct{}

var _ = tc.Suite(&monitorSuite{})

func (s *monitorSuite) TestQueryMonitor(c *tc.C) {
	transientDir := c.MkDir()
	mon := reboot.NewMonitor(transientDir)

	unit, err := names.ParseUnitTag("unit-wordpress-0")
	c.Assert(err, tc.ErrorIsNil)

	// Since we pointed the monitor to an empty dir, querying it should
	// return true (reboot detected) and the flag file will be created.
	rebootDetected, err := mon.Query(unit)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rebootDetected, tc.IsTrue)

	// Querying the monitor a second time should return false as we
	// already processed the reboot notification
	rebootDetected, err = mon.Query(unit)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rebootDetected, tc.IsFalse, tc.Commentf("got unexpected reboot notification"))

	// If we purge the monitor's state for this entity we can query it
	// again and get a reboot detected notification.
	err = mon.PurgeState(unit)
	c.Assert(err, tc.ErrorIsNil)

	rebootDetected, err = mon.Query(unit)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rebootDetected, tc.IsTrue)
}

func (s *monitorSuite) TestQueryMonitorForDifferentEntities(c *tc.C) {
	transientDir := c.MkDir()
	mon := reboot.NewMonitor(transientDir)

	unit1, err := names.ParseUnitTag("unit-wordpress-0")
	c.Assert(err, tc.ErrorIsNil)
	unit2, err := names.ParseUnitTag("unit-mysql-0")
	c.Assert(err, tc.ErrorIsNil)

	// Querying for different entities with no prior monitor state should
	// yield a reboot notification for each entity.
	rebootDetected, err := mon.Query(unit1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rebootDetected, tc.IsTrue)

	rebootDetected, err = mon.Query(unit2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rebootDetected, tc.IsTrue)

	// Querying the monitor a second time should return false as we
	// already processed the reboot notification for both entities
	rebootDetected, err = mon.Query(unit1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rebootDetected, tc.IsFalse, tc.Commentf("got unexpected reboot notification"))
	rebootDetected, err = mon.Query(unit2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rebootDetected, tc.IsFalse, tc.Commentf("got unexpected reboot notification"))
}
func TestAll(t *testing.T) {
	tc.TestingT(t)
}
