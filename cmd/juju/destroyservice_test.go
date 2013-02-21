package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type DestroyServiceSuite struct {
	repoSuite
}

var _ = Suite(&DestroyServiceSuite{})

func runDestroyService(c *C, args ...string) error {
	return testing.RunCommand(c, &DestroyServiceCommand{}, args)
}

func (s *DestroyServiceSuite) TestSuccess(c *C) {
	// Destroy a service that exists.
	testing.Charms.BundlePath(s.seriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
	err = runDestroyService(c, "riak")
	c.Assert(err, IsNil)
	riak, err := s.State.Service("riak")
	c.Assert(err, IsNil)
	c.Assert(riak.Life(), Equals, state.Dying)
}

func (s *DestroyServiceSuite) TestFailure(c *C) {
	// Destroy a service that does not exist.
	err := runDestroyService(c, "gargleblaster")
	c.Assert(err, ErrorMatches, `service "gargleblaster" not found`)
}

func (s *DestroyServiceSuite) TestInvalidArgs(c *C) {
	err := runDestroyService(c)
	c.Assert(err, ErrorMatches, `no service specified`)
	err = runDestroyService(c, "ping", "pong")
	c.Assert(err, ErrorMatches, `unrecognized args: \["pong"\]`)
	err = runDestroyService(c, "invalid:name")
	c.Assert(err, ErrorMatches, `invalid service name "invalid:name"`)
}
