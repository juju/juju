// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testcharms"
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

func (s *RemoveRelationSuite) setupRelationForRemove(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, jc.ErrorIsNil)
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err = runDeploy(c, "local:logging", "logging")
	c.Assert(err, jc.ErrorIsNil)
	runAddRelation(c, "riak", "logging")
}

func (s *RemoveRelationSuite) TestRemoveRelation(c *gc.C) {
	s.setupRelationForRemove(c)

	// Destroy a relation that exists.
	err := runRemoveRelation(c, "logging", "riak")
	c.Assert(err, jc.ErrorIsNil)

	// Destroy a relation that used to exist.
	err = runRemoveRelation(c, "riak", "logging")
	c.Assert(err, gc.ErrorMatches, `relation "logging:info riak:juju-info" not found`)

	// Invalid removes.
	err = runRemoveRelation(c, "ping", "pong")
	c.Assert(err, gc.ErrorMatches, `service "ping" not found`)
	err = runRemoveRelation(c, "riak")
	c.Assert(err, gc.ErrorMatches, `a relation must involve two services`)
}

func (s *RemoveRelationSuite) TestBlockRemoveRelation(c *gc.C) {
	s.setupRelationForRemove(c)

	// block operation
	s.AssertConfigParameterUpdated(c, "block-remove-object", true)
	// Destroy a relation that exists.
	err := runRemoveRelation(c, "logging", "riak")
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To unblock removal.*")
}
