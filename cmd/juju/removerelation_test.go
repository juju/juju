// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd/envcmd"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

type RemoveRelationSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&RemoveRelationSuite{})

func runRemoveRelation(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveRelationCommand{}), args)
	return err
}

func (s *RemoveRelationSuite) TestRemoveRelation(c *gc.C) {
	testing.Charms.BundlePath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, gc.IsNil)
	testing.Charms.BundlePath(s.SeriesPath, "logging")
	err = runDeploy(c, "local:logging", "logging")
	c.Assert(err, gc.IsNil)
	runAddRelation(c, "riak", "logging")

	// Destroy a relation that exists.
	err = runRemoveRelation(c, "logging", "riak")
	c.Assert(err, gc.IsNil)

	// Destroy a relation that used to exist.
	err = runRemoveRelation(c, "riak", "logging")
	c.Assert(err, gc.ErrorMatches, `relation "logging:info riak:juju-info" not found`)

	// Invalid removes.
	err = runRemoveRelation(c, "ping", "pong")
	c.Assert(err, gc.ErrorMatches, `service "ping" not found`)
	err = runRemoveRelation(c, "riak")
	c.Assert(err, gc.ErrorMatches, `a relation must involve two services`)
}
