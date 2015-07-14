// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type RemoveUnitSuite struct {
	jujutesting.RepoSuite
	CmdBlockHelper
}

func (s *RemoveUnitSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&RemoveUnitSuite{})

func runRemoveUnit(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&RemoveUnitCommand{}), args...)
	return err
}

func (s *RemoveUnitSuite) setupUnitForRemove(c *gc.C) *state.Service {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "-n", "2", "local:dummy", "dummy")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL(fmt.Sprintf("local:%s/dummy-1", testing.FakeDefaultSeries))
	svc, _ := s.AssertService(c, "dummy", curl, 2, 0)
	return svc
}

func (s *RemoveUnitSuite) TestRemoveUnit(c *gc.C) {
	svc := s.setupUnitForRemove(c)

	err := runRemoveUnit(c, "dummy/0", "dummy/1", "dummy/2", "sillybilly/17")
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "dummy/2" does not exist; unit "sillybilly/17" does not exist`)
	units, err := svc.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, u := range units {
		c.Assert(u.Life(), gc.Equals, state.Dying)
	}
}
func (s *RemoveUnitSuite) TestBlockRemoveUnit(c *gc.C) {
	svc := s.setupUnitForRemove(c)

	// block operation
	s.BlockRemoveObject(c, "TestBlockRemoveUnit")
	err := runRemoveUnit(c, "dummy/0", "dummy/1")
	s.AssertBlocked(c, err, ".*TestBlockRemoveUnit.*")
	c.Assert(svc.Life(), gc.Equals, state.Alive)
}
