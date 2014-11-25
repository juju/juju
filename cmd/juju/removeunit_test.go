// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type RemoveUnitSuite struct {
	jujutesting.RepoSuite
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
	s.AssertConfigParameterUpdated(c, "block-remove-object", true)
	err := runRemoveUnit(c, "dummy/0", "dummy/1")
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
	c.Assert(svc.Life(), gc.Equals, state.Alive)

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To unblock removal.*")
}
