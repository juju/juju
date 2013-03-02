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
	about     string
	service   string
	numUnits  int
	err       string
	unitCount int
}{
	{
		about:    "unknown service name",
		service:  "unknown-service",
		numUnits: 1,
		err:      `service "unknown-service" not found`
	}
}

func (s *AddUnitSuite) TestServiceAddUnit(c *C) {
	var err
	charm := s.AddTestingCharm(c, "dummy")
	svc, err = s.State.AddService("dummy-service", charm)
	c.Assert(err, isNil)

	err = svc.Destroy()
	c.Assert(err, isNil)
}
