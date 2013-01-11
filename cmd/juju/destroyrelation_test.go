package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	_ "launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type DestroyRelationSuite struct {
	repoSuite
}

var _ = Suite(&DestroyRelationSuite{})

func runDestroyRelation(c *C, args ...string) error {
	com := &DestroyRelationCommand{}
	if err := com.Init(newFlagSet(), args); err != nil {
		return err
	}
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *DestroyRelationSuite) TestDestroyRelation(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "series", "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
	testing.Charms.BundlePath(s.seriesPath, "series", "logging")
	err = runDeploy(c, "local:logging", "logging")
	c.Assert(err, IsNil)
	runAddRelation(c, "riak", "logging")

	// Destroy a relation that exists.
	err = runDestroyRelation(c, "logging", "riak")
	c.Assert(err, IsNil)

	// Destroy a relation that used to exist.
	err = runDestroyRelation(c, "riak", "logging")
	c.Assert(err, ErrorMatches, `relation "logging:info riak:juju-info" not found`)

	// Invalid removes.
	err = runDestroyRelation(c, "ping", "pong")
	c.Assert(err, ErrorMatches, `service "ping" not found`)
	err = runDestroyRelation(c, "riak")
	c.Assert(err, ErrorMatches, `a relation must involve two services`)
}
