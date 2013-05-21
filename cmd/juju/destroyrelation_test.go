// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	. "launchpad.net/gocheck"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

type DestroyRelationSuite struct {
	jujutesting.RepoSuite
}

var _ = Suite(&DestroyRelationSuite{})

func runDestroyRelation(c *C, args ...string) error {
	_, err := testing.RunCommand(c, &DestroyRelationCommand{}, args)
	return err
}

func (s *DestroyRelationSuite) TestDestroyRelation(c *C) {
	testing.Charms.BundlePath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
	testing.Charms.BundlePath(s.SeriesPath, "logging")
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
