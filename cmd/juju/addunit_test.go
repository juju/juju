package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

type AddUnitSuite struct {
	jujutesting.RepoSuite
}

var _ = Suite(&AddUnitSuite{})

func runAddUnit(c *C, args ...string) error {
	_, err := testing.RunCommand(c, &AddUnitCommand{}, args)
	return err
}

func (s *AddUnitSuite) TestAddUnit(c *C) {
	testing.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)

	err = runAddUnit(c, "some-service-name")
	c.Assert(err, IsNil)
	s.AssertService(c, "some-service-name", curl, 2, 0)

	err = runAddUnit(c, "--num-units", "2", "some-service-name")
	c.Assert(err, IsNil)
	s.AssertService(c, "some-service-name", curl, 4, 0)
}
