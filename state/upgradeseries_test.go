// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
)

type UpgradeSeriesSuite struct {
	ConnSuite

	machine0 *state.Machine
	machine1 *state.Machine
}

var _ = gc.Suite(&UpgradeSeriesSuite{})

// SetUpTest adds two machines with units that have a single
// application in common.
func (s *UpgradeSeriesSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	var err error
	s.machine0, err = s.State.AddMachine("trusty", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.machine1, err = s.State.AddMachine("trusty", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	ch := state.AddTestingCharmMultiSeries(c, s.State, "wordpress")
	app := state.AddTestingApplicationForSeries(c, s.State, "trusty", "wordpress", ch)

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine0)
	c.Assert(err, jc.ErrorIsNil)

	unit, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine1)
	c.Assert(err, jc.ErrorIsNil)

	ch = state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app = state.AddTestingApplicationForSeries(c, s.State, "trusty", "multi-series", ch)

	unit, err = app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machine0)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeSeriesSuite) TestUpgradeSeriesApplicationIntersectNoLockError(c *gc.C) {
	s.lockMachine1(c)
	_, err := s.State.UpgradeSeriesApplicationIntersect(s.machine0.Id())
	c.Assert(err, gc.ErrorMatches, `upgrade lock for machine "0" not found`)
}

func (s *UpgradeSeriesSuite) TestUpgradeSeriesApplicationIntersectCompletedLockNoIntersect(c *gc.C) {
	s.lockMachine0(c)
	s.lockMachine1(c)

	err := s.machine1.SetUpgradeSeriesStatus(model.UpgradeSeriesCompleted, "finished, but lock not deleted")
	c.Assert(err, jc.ErrorIsNil)

	apps, err := s.State.UpgradeSeriesApplicationIntersect(s.machine0.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apps, gc.HasLen, 0)
}

func (s *UpgradeSeriesSuite) TestUpgradeSeriesApplicationIntersectSuccess(c *gc.C) {
	s.lockMachine0(c)
	s.lockMachine1(c)

	apps, err := s.State.UpgradeSeriesApplicationIntersect(s.machine0.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(apps, jc.SameContents, []string{"wordpress"})
}

func (s *UpgradeSeriesSuite) lockMachine0(c *gc.C) {
	err := s.machine0.CreateUpgradeSeriesLock([]string{"multi-series/0", "wordpress/0"}, "xenial")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeSeriesSuite) lockMachine1(c *gc.C) {
	err := s.machine1.CreateUpgradeSeriesLock([]string{"wordpress/1"}, "xenial")
	c.Assert(err, jc.ErrorIsNil)
}
