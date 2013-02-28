package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
)

type ExposeSuite struct {
	repoSuite
}

var _ = Suite(&ExposeSuite{})

func runExpose(c *C, args ...string) error {
	return testing.RunCommand(c, &ExposeCommand{}, args)
}

func (s *ExposeSuite) assertExposed(c *C, service string) {
	svc, err := s.State.Service(service)
	c.Assert(err, IsNil)
	exposed := svc.IsExposed()
	c.Assert(exposed, Equals, true)
}

func (s *ExposeSuite) TestExpose(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.assertService(c, "some-service-name", curl, 1, 0)

	err = runExpose(c, "some-service-name")
	c.Assert(err, IsNil)
	s.assertExposed(c, "some-service-name")

	err = runExpose(c, "nonexistent-service")
	c.Assert(err, ErrorMatches, `service "nonexistent-service" not found`)
}
