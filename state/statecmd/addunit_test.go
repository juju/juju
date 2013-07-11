// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

type AddUnitsSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&AddUnitsSuite{})

var addUnitsTests = []struct {
	about          string
	service        string
	numUnits       int
	forceMachineId string
	err            string
	expectedUnits  int
}{
	{
		about:    "unknown service name",
		service:  "unknown-service",
		numUnits: 1,
		err:      `service "unknown-service" not found`,
	},
	{
		about:    "add zero units",
		service:  "dummy-service",
		numUnits: 0,
		err:      "must add at least one unit",
	},
	{
		about:         "add one unit",
		service:       "dummy-service",
		numUnits:      1,
		expectedUnits: 1,
	},
	{
		about:         "add multiple units",
		service:       "dummy-service",
		numUnits:      5,
		expectedUnits: 6,
	},
	{
		about:          "add multiple units with force machine",
		service:        "dummy-service",
		numUnits:       5,
		forceMachineId: "0",
		err:            "cannot use --num-units with --force-machine",
	},
}

func (s *AddUnitsSuite) TestAddServiceUnits(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("dummy-service", charm)
	c.Assert(err, IsNil)

	for i, t := range addUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		units, err := statecmd.AddServiceUnits(s.State, params.AddServiceUnits{
			ServiceName:    t.service,
			ForceMachineId: t.forceMachineId,
			NumUnits:       t.numUnits,
		})
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			c.Assert(err, IsNil)
			c.Assert(units, HasLen, t.numUnits)
			for _, unit := range units {
				c.Assert(unit.ServiceName(), Equals, t.service)
			}
			service, err := s.State.Service(t.service)
			c.Assert(err, IsNil)
			unitCount, err := service.AllUnits()
			c.Assert(err, IsNil)
			c.Assert(len(unitCount), Equals, t.expectedUnits)
		}
	}

	err = svc.Destroy()
	c.Assert(err, IsNil)
}
