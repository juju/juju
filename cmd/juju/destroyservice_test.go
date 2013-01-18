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

func (s *DestroyServiceSuite) TestDestroyRelation(c *C) {
	// Create two services with a relation between them.
	testing.Charms.BundlePath(s.seriesPath, "series", "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
	testing.Charms.BundlePath(s.seriesPath, "series", "logging")
	err = runDeploy(c, "local:logging", "logging")
	c.Assert(err, IsNil)
	runAddRelation(c, "riak", "logging")

	// Get the state entities to allow sane testing.
	logging, err := s.State.Service("logging")
	c.Assert(err, IsNil)
	rels, err := logging.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 1)
	rel := rels[0]

	// Destroy a service that exists; check the service and relation are
	// destroyed. (No need to test make-Dying behaviour here, I think.)
	err = runDestroyService(c, "logging")
	c.Assert(err, IsNil)
	err = logging.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
	err = rel.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)

	// Destroy a service that does not exist.
	err = runDestroyService(c, "gargleblaster")
	c.Assert(err, ErrorMatches, `service "gargleblaster" not found`)

	// Invalid args.
	err = runDestroyService(c)
	c.Assert(err, ErrorMatches, `no service specified`)
	err = runDestroyService(c, "ping", "pong")
	c.Assert(err, ErrorMatches, `unrecognized args: \["pong"\]`)
	err = runDestroyService(c, "invalid:name")
	c.Assert(err, ErrorMatches, `invalid service name "invalid:name"`)
}
