// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	charmtesting "github.com/juju/juju/charm/testing"
	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

type RemoveRelationSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&RemoveRelationSuite{})

func runRemoveRelation(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveRelationCommand{}), args...)
	return err
}

func (s *RemoveRelationSuite) TestRemoveRelation(c *gc.C) {
	charmtesting.Charms.BundlePath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, gc.IsNil)
	charmtesting.Charms.BundlePath(s.SeriesPath, "logging")
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
