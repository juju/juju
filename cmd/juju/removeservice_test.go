// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd/envcmd"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
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
	testing.Charms.BundlePath(s.SeriesPath, "riak")
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
