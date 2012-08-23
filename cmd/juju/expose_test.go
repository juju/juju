package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type ExposeSuite struct {
	DeploySuite
}

var _ = Suite(&ExposeSuite{})

func (s *ExposeSuite) SetUpTest(c *C) {
	s.DeploySuite.SetUpTest(c)
}

func (s *ExposeSuite) TearDownTest(c *C) {
	s.DeploySuite.TearDownTest(c)
}

func runExpose(c *C, args ...string) error {
	com := &ExposeCommand{}
	err := com.Init(newFlagSet(), args)
	c.Assert(err, IsNil)
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *ExposeSuite) assertExposed(c *C, service string) {
	svc, err := s.State.Service(service)
	c.Assert(err, IsNil)
	exposed, err := svc.IsExposed()
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
	c.Assert(err, ErrorMatches, `.*service with name "nonexistent-service" not found`)
}
