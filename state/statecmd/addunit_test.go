// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

type AddUnitsSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&AddUnitsSuite{})

var addUnitsTests = []struct {
	about            string
	service          string
	numUnits         int
	forceMachineSpec string
	err              string
	expectedUnits    int
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
		about:            "add multiple units with force machine",
		service:          "dummy-service",
		numUnits:         5,
		forceMachineSpec: "0",
		err:              "cannot use --num-units with --to",
	},
}

func (s *AddUnitsSuite) TestAddServiceUnits(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("dummy-service", charm)
	c.Assert(err, gc.IsNil)

	for i, t := range addUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		units, err := statecmd.AddServiceUnits(s.State, params.AddServiceUnits{
			ServiceName:   t.service,
			ToMachineSpec: t.forceMachineSpec,
			NumUnits:      t.numUnits,
		})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(units, gc.HasLen, t.numUnits)
			for _, unit := range units {
				c.Assert(unit.ServiceName(), gc.Equals, t.service)
			}
			service, err := s.State.Service(t.service)
			c.Assert(err, gc.IsNil)
			unitCount, err := service.AllUnits()
			c.Assert(err, gc.IsNil)
			c.Assert(len(unitCount), gc.Equals, t.expectedUnits)
		}
	}

	err = svc.Destroy()
	c.Assert(err, gc.IsNil)
}
