package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type UnexposeSuite struct {
	repoSuite
}

var _ = Suite(&UnexposeSuite{})

func runUnexpose(c *C, args ...string) error {
	com := &UnexposeCommand{}
	err := com.Init(newFlagSet(), args)
	c.Assert(err, IsNil)
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *UnexposeSuite) assertExposed(c *C, service string, expected bool) {
	svc, err := s.State.Service(service)
	c.Assert(err, IsNil)
	actual, err := svc.IsExposed()
	c.Assert(actual, Equals, expected)
}

func (s *UnexposeSuite) TestUnexpose(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "series", "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.assertService(c, "some-service-name", curl, 1, 0)

	err = runExpose(c, "some-service-name")
	c.Assert(err, IsNil)
	s.assertExposed(c, "some-service-name", true)

	err = runUnexpose(c, "some-service-name")
	c.Assert(err, IsNil)
	s.assertExposed(c, "some-service-name", false)

	err = runUnexpose(c, "nonexistent-service")
	c.Assert(err, ErrorMatches, `.*service with name "nonexistent-service" not found`)
}
