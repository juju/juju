// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

type DestroySuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&DestroySuite{})

var serviceDestroyTests = []struct {
	about   string
	service string
	err     string
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:   "destroy a service",
		service: "dummy-service",
	},
	{
		about:   "destroy an already destroyed service",
		service: "dummy-service",
		err:     `service "dummy-service" not found`,
	},
}

func (s *DestroySuite) TestServiceDestroy(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy-service", charm)
	c.Assert(err, gc.IsNil)
	c.Assert(svc.Life(), gc.Equals, state.Alive)
	c.Logf("Svc: %+v", svc)

	for i, t := range serviceDestroyTests {
		c.Logf("test %d. %s", i, t.about)
		err = statecmd.ServiceDestroy(s.State, params.ServiceDestroy{
			ServiceName: t.service,
		})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, gc.IsNil)
		}
	}
}
