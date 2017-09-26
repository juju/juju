// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type RemoveUnitSuite struct {
	jujutesting.RepoSuite
	testing.CmdBlockHelper
}

func (s *RemoveUnitSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = testing.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&RemoveUnitSuite{})

func runRemoveUnit(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, NewRemoveUnitCommand(), args...)
}

func (s *RemoveUnitSuite) setupUnitForRemove(c *gc.C) *state.Application {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	_, err := runDeploy(c, "-n", "2", ch, "multi-series", "--series", "precise")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:precise/multi-series-1")
	svc, _ := s.AssertService(c, "multi-series", curl, 2, 0)
	return svc
}

func (s *RemoveUnitSuite) TestRemoveUnit(c *gc.C) {
	app := s.setupUnitForRemove(c)

	ctx, err := runRemoveUnit(c, "multi-series/0", "multi-series/1", "multi-series/2", "sillybilly/17")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
removing unit multi-series/0
removing unit multi-series/1
removing unit multi-series/2 failed: unit "multi-series/2" does not exist
removing unit sillybilly/17 failed: unit "sillybilly/17" does not exist
`[1:])

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, u := range units {
		c.Assert(u.Life(), gc.Equals, state.Dying)
	}
}

func (s *RemoveUnitSuite) TestRemoveUnitDetachesStorage(c *gc.C) {
	s.testRemoveUnitRemoveStorage(c, false)
}

func (s *RemoveUnitSuite) TestRemoveUnitDestroyStorage(c *gc.C) {
	s.testRemoveUnitRemoveStorage(c, true)
}

func (s *RemoveUnitSuite) testRemoveUnitRemoveStorage(c *gc.C, destroy bool) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "storage-filesystem-multi-series")
	_, err := runDeploy(c, ch, "storage-filesystem", "--storage", "data=modelscoped,2")
	c.Assert(err, jc.ErrorIsNil)

	// Materialise the storage by assigning the unit to a machine.
	u, err := s.State.Unit("storage-filesystem/0")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(u, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	args := []string{"storage-filesystem/0"}
	action := "detach"
	if destroy {
		args = append(args, "--destroy-storage")
		action = "remove"
	}
	ctx, err := runRemoveUnit(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, fmt.Sprintf(`
removing unit storage-filesystem/0
- will %[1]s storage data/0
- will %[1]s storage data/1
`[1:], action))
}

func (s *RemoveUnitSuite) TestBlockRemoveUnit(c *gc.C) {
	app := s.setupUnitForRemove(c)

	// block operation
	s.BlockRemoveObject(c, "TestBlockRemoveUnit")
	_, err := runRemoveUnit(c, "dummy/0", "dummy/1")
	s.AssertBlocked(c, err, ".*TestBlockRemoveUnit.*")
	c.Assert(app.Life(), gc.Equals, state.Alive)
}
