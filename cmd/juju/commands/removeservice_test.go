// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type RemoveServiceSuite struct {
	jujutesting.RepoSuite
	CmdBlockHelper
}

var _ = gc.Suite(&RemoveServiceSuite{})

func (s *RemoveServiceSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

func runRemoveService(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveServiceCommand{}), args...)
	return err
}

func (s *RemoveServiceSuite) setupTestService(c *gc.C) {
	// Destroy a service that exists.
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveServiceSuite) TestSuccess(c *gc.C) {
	s.setupTestService(c)
	err := runRemoveService(c, "riak")
	c.Assert(err, jc.ErrorIsNil)
	riak, err := s.State.Service("riak")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(riak.Life(), gc.Equals, state.Dying)
}

func (s *RemoveServiceSuite) TestBlockRemoveService(c *gc.C) {
	s.setupTestService(c)

	// block operation
	s.BlockRemoveObject(c, "TestBlockRemoveService")
	err := runRemoveService(c, "riak")
	s.AssertBlocked(c, err, ".*TestBlockRemoveService.*")
	riak, err := s.State.Service("riak")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(riak.Life(), gc.Equals, state.Alive)
}

func (s *RemoveServiceSuite) TestFailure(c *gc.C) {
	// Destroy a service that does not exist.
	err := runRemoveService(c, "gargleblaster")
	c.Assert(err, gc.ErrorMatches, `service "gargleblaster" not found`)
}

func (s *RemoveServiceSuite) TestInvalidArgs(c *gc.C) {
	err := runRemoveService(c)
	c.Assert(err, gc.ErrorMatches, `no service specified`)
	err = runRemoveService(c, "ping", "pong")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["pong"\]`)
	err = runRemoveService(c, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid service name "invalid:name"`)
}
