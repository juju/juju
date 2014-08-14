// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	charmtesting "gopkg.in/juju/charm.v3/testing"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type RemoveServiceSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&RemoveServiceSuite{})

func runRemoveService(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveServiceCommand{}), args...)
	return err
}

func (s *RemoveServiceSuite) TestSuccess(c *gc.C) {
	// Destroy a service that exists.
	charmtesting.Charms.CharmArchivePath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, gc.IsNil)
	err = runRemoveService(c, "riak")
	c.Assert(err, gc.IsNil)
	riak, err := s.State.Service("riak")
	c.Assert(err, gc.IsNil)
	c.Assert(riak.Life(), gc.Equals, state.Dying)
}

func (s *RemoveServiceSuite) TestFailure(c *gc.C) {
	// Destroy a service that does not exist.
	err := runRemoveService(c, "gargleblaster")
	c.Assert(err, gc.ErrorMatches, `service "gargleblaster" not found`)
}

func (s *RemoveServiceSuite) TestInvalidArgs(c *gc.C) {
	err := runRemoveService(c)
	c.Assert(err, gc.ErrorMatches, `no service specified`)
	err = runRemoveService(c, "ping", "pong")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["pong"\]`)
	err = runRemoveService(c, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid service name "invalid:name"`)
}
