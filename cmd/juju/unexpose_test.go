// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd/envcmd"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

type UnexposeSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&UnexposeSuite{})

func runUnexpose(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&UnexposeCommand{}), args)
	return err
}

func (s *UnexposeSuite) assertExposed(c *gc.C, service string, expected bool) {
	svc, err := s.State.Service(service)
	c.Assert(err, gc.IsNil)
	actual := svc.IsExposed()
	c.Assert(actual, gc.Equals, expected)
}

func (s *UnexposeSuite) TestUnexpose(c *gc.C) {
	testing.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)

	err = runExpose(c, "some-service-name")
	c.Assert(err, gc.IsNil)
	s.assertExposed(c, "some-service-name", true)

	err = runUnexpose(c, "some-service-name")
	c.Assert(err, gc.IsNil)
	s.assertExposed(c, "some-service-name", false)

	err = runUnexpose(c, "nonexistent-service")
	c.Assert(err, gc.ErrorMatches, `service "nonexistent-service" not found`)
}
