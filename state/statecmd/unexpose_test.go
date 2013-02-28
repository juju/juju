package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/statecmd"
)

type UnexposeSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&UnexposeSuite{})

var serviceUnexposeTests = []struct {
	about            string
	service          string
	err              string
	initialExposure  bool
	expectedExposure bool
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:            "unexpose a service",
		service:          "dummy-service",
		initialExposure:  true,
		expectedExposure: false,
	},
	{
		about:            "unexpose an already unexposed service",
		service:          "dummy-service",
		initialExposure:  false,
		expectedExposure: false,
	},
}

func (s *UnexposeSuite) TestServiceUnexpose(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	for i, t := range serviceUnexposeTests {
		c.Logf("test %d. %s", i, t.about)
		svc, err := s.State.AddService("dummy-service", charm)
		c.Assert(err, IsNil)
		if t.initialExposure {
			svc.SetExposed()
		}
		c.Assert(svc.IsExposed(), Equals, t.initialExposure)
		params := statecmd.ServiceUnexposeParams{ServiceName: t.service}
		err = statecmd.ServiceUnexpose(s.State, params)
		if t.err == "" {
			c.Assert(err, IsNil)
			svc.Refresh()
			c.Assert(svc.IsExposed(), Equals, t.expectedExposure)
		} else {
			c.Assert(err, ErrorMatches, t.err)
		}
		err = svc.Destroy()
		c.Assert(err, IsNil)
	}
}
