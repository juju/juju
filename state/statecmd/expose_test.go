package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/statecmd"
)

type ExposeSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&ExposeSuite{})

var serviceExposeTests = []struct {
	about   string
	service string
	err     string
	exposed bool
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		// Exposing a known service succeeds, so err is "".
		about:   "expose a service",
		service: "dummy-service",
		exposed: true,
	},
}

func (suite *ExposeSuite) TestServiceExpose(c *C) {
	charm := suite.AddTestingCharm(c, "dummy")
	for i, t := range serviceExposeTests {
		c.Logf("test %d. %s", i, t.about)
		svc, err := suite.State.AddService("dummy-service", charm)
		c.Assert(err, IsNil)
		c.Assert(svc.IsExposed(), Equals, false)
		err = statecmd.ServiceExpose(suite.State, statecmd.ServiceExposeParams{
			ServiceName: t.service,
		})
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			c.Assert(err, IsNil)
			service, err := suite.State.Service(t.service)
			c.Assert(err, IsNil)
			c.Assert(service.IsExposed(), Equals, t.exposed)
		}
		err = svc.Destroy()
		c.Assert(err, IsNil)
	}
}
