package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type AddUnitSuite struct {
	repoSuite
}

var _ = Suite(&AddUnitSuite{})

func runAddUnit(c *C, args ...string) error {
	com := &AddUnitCommand{}
	err := com.Init(newFlagSet(), args)
	c.Assert(err, IsNil)
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *AddUnitSuite) TestAddUnit(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "dummy", "series")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.assertService(c, "some-service-name", curl, 1, 0)

	err = runAddUnit(c, "some-service-name")
	c.Assert(err, IsNil)
	s.assertService(c, "some-service-name", curl, 2, 0)

	err = runAddUnit(c, "--num-units", "2", "some-service-name")
	c.Assert(err, IsNil)
	s.assertService(c, "some-service-name", curl, 4, 0)
}
