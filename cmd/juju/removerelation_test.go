package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	_ "launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type RemoveRelationSuite struct {
	repoSuite
}

var _ = Suite(&RemoveRelationSuite{})

func runRemoveRelation(c *C, args ...string) error {
	com := &RemoveRelationCommand{}
	if err := com.Init(newFlagSet(), args); err != nil {
		return err
	}
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *RemoveRelationSuite) TestRemoveRelation(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "series", "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
	testing.Charms.BundlePath(s.seriesPath, "series", "logging")
	err = runDeploy(c, "local:logging", "logging")
	c.Assert(err, IsNil)
	runAddRelation(c, "riak", "logging")

	// Remove a relation that exists.
	err = runRemoveRelation(c, "logging", "riak")
	c.Assert(err, IsNil)

	// Remove a relation that used to exist.
	err = runRemoveRelation(c, "riak", "logging")
	c.Assert(err, ErrorMatches, `relation "logging:info riak:juju-info" not found`)

	// Invalid removes.
	err = runRemoveRelation(c, "ping", "pong")
	c.Assert(err, ErrorMatches, `service "ping" not found`)
	err = runRemoveRelation(c, "riak")
	c.Assert(err, ErrorMatches, `a relation must involve two services`)
}
