// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type DestroyServiceSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&DestroyServiceSuite{})

func runDestroyService(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, &DestroyServiceCommand{}, args)
	return err
}

func (s *DestroyServiceSuite) TestSuccess(c *gc.C) {
	// Destroy a service that exists.
	testing.Charms.BundlePath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, gc.IsNil)
	err = runDestroyService(c, "riak")
	c.Assert(err, gc.IsNil)
	riak, err := s.State.Service("riak")
	c.Assert(err, gc.IsNil)
	c.Assert(riak.Life(), gc.Equals, state.Dying)
}

func (s *DestroyServiceSuite) TestFailure(c *gc.C) {
	// Destroy a service that does not exist.
	err := runDestroyService(c, "gargleblaster")
	c.Assert(err, gc.ErrorMatches, `service "gargleblaster" not found`)
}

func (s *DestroyServiceSuite) TestInvalidArgs(c *gc.C) {
	err := runDestroyService(c)
	c.Assert(err, gc.ErrorMatches, `no service specified`)
	err = runDestroyService(c, "ping", "pong")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["pong"\]`)
	err = runDestroyService(c, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid service name "invalid:name"`)
}
