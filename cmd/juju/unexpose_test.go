// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"gopkg.in/juju/charm.v3"
	charmtesting "gopkg.in/juju/charm.v3/testing"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

type UnexposeSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&UnexposeSuite{})

func runUnexpose(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&UnexposeCommand{}), args...)
	return err
}

func (s *UnexposeSuite) assertExposed(c *gc.C, service string, expected bool) {
	svc, err := s.State.Service(service)
	c.Assert(err, gc.IsNil)
	actual := svc.IsExposed()
	c.Assert(actual, gc.Equals, expected)
}

func (s *UnexposeSuite) TestUnexpose(c *gc.C) {
	charmtesting.Charms.CharmArchivePath(s.SeriesPath, "dummy")
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
