// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/common"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type RemoveRelationSuite struct {
	jujutesting.RepoSuite
	common.CmdBlockHelper
}

func (s *RemoveRelationSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = common.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&RemoveRelationSuite{})

func runRemoveRelation(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, NewRemoveRelationCommand(), args...)
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
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `relation "logging:info riak:juju-info" not found`,
		Code:    "not found",
	})

	// Invalid removes.
	err = runRemoveRelation(c, "ping", "pong")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `service "ping" not found`,
		Code:    "not found",
	})
	err = runRemoveRelation(c, "riak")
	c.Assert(err, gc.ErrorMatches, `a relation must involve two services`)
}

func (s *RemoveRelationSuite) TestBlockRemoveRelation(c *gc.C) {
	s.setupRelationForRemove(c)

	// block operation
	s.BlockRemoveObject(c, "TestBlockRemoveRelation")
	// Destroy a relation that exists.
	err := runRemoveRelation(c, "logging", "riak")
	s.AssertBlocked(c, err, ".*TestBlockRemoveRelation.*")
}
