package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type DestroyServiceSuite struct {
	repoSuite
}

var _ = Suite(&DestroyServiceSuite{})

func runDestroyService(c *C, args ...string) error {
	com := &DestroyServiceCommand{}
	if err := com.Init(newFlagSet(), args); err != nil {
		return err
	}
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
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
