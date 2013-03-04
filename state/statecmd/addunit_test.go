package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/statecmd"
)

type AddUnitSuite struct {
	testing.JujuConnSuite
}

// Run-time check to ensure AddUnitSuite implements the Suite interface.
var _ = Suite(&AddUnitSuite{})

var addUnitTests = []struct {
	about         string
	service       string
	numUnits      int
	err           string
	expectedUnits int
}{
	{
		about:    "unknown service name",
		service:  "unknown-service",
		numUnits: 1,
		err:      `service "unknown-service" not found`,
	},
	{
		about: "add negative units",
		service: "dummy-service",
		numUnits: -1,
		err: `must add at least one unit`,
	},
	{
		about: "add one unit",
		service: "dummy-service",
		numUnits: 1,
		expectedUnits: 2,
	},
	{
		about: "add multiple units",
		service: "dummy-service",
		numUnits: 5,
		expectedUnits: 7,
	},
}

func (s *AddUnitSuite) TestServiceAddUnit(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	svc, err = s.State.AddService("dummy-service", charm)
	c.Assert(err, isNil)

	for i, t := range addUnitTests {
		c.Logf("test %d. %s", i, t.about)
		err = statecmd.ServiceAddUnits(s.State, statecmd.ServiceAddUnitParams{
			ServiceName: t.service,
			NumUnits: t.numUnits,
		})
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			c.assert(err, IsNil)
			service, err :=  s.State.Service(t.service)
			c.Assert(err, IsNil)
			c.Assert(len(service.AllUnits()), Equals, t.expectedUnits)
		}
	}

	err = svc.Destroy()
	c.Assert(err, isNil)
}
