// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/upgradeseries"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/testing"
)

type upgradeSeriesSuite struct {
	jujutesting.BaseSuite

	tag  names.Tag
	args params.Entities
}

var _ = gc.Suite(&upgradeSeriesSuite{})

func (s *upgradeSeriesSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0")
	s.args = params.Entities{Entities: []params.Entity{{Tag: s.tag.String()}}}

	s.BaseSuite.SetUpTest(c)
}

func (s *upgradeSeriesSuite) TestMachineStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	resultSource := params.UpgradeSeriesStatusResultsNew{
		Results: []params.UpgradeSeriesStatusResultNew{{
			Status: params.UpgradeSeriesStatus{Status: model.PrepareStarted, Entity: s.args.Entities[0]}},
		},
	}
	fCaller.EXPECT().FacadeCall("MachineStatus", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	status, err := api.MachineStatus()
	c.Assert(err, gc.IsNil)
	c.Check(status, gc.Equals, model.PrepareStarted)
}

func (s *upgradeSeriesSuite) TestMachineStatusNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	resultSource := params.UpgradeSeriesStatusResultsNew{
		Results: []params.UpgradeSeriesStatusResultNew{{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "did not find",
			},
		}},
	}
	fCaller.EXPECT().FacadeCall("MachineStatus", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	status, err := api.MachineStatus()
	c.Assert(err, gc.ErrorMatches, "did not find")
	c.Check(errors.IsNotFound(err), jc.IsTrue)
	c.Check(string(status), gc.Equals, "")
}

func (s *upgradeSeriesSuite) TestSetMachineStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatus{
			{Status: model.CompleteStarted, Entity: s.args.Entities[0]},
		},
	}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	fCaller.EXPECT().FacadeCall("SetMachineStatus", args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	err := api.SetMachineStatus(model.CompleteStarted)
	c.Assert(err, gc.IsNil)
}

func (s *upgradeSeriesSuite) TestUnitsPrepared(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	r0 := "redis/0"
	r1 := "redis/1"

	resultSource := params.EntitiesResults{
		Results: []params.EntitiesResult{{Entities: []params.Entity{
			{Tag: r0},
			{Tag: r1},
		}}},
	}
	fCaller.EXPECT().FacadeCall("UnitsPrepared", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	units, err := api.UnitsPrepared()
	c.Assert(err, gc.IsNil)

	expected := []names.UnitTag{names.NewUnitTag(r0), names.NewUnitTag(r1)}
	c.Check(units, jc.SameContents, expected)
}

func (s *upgradeSeriesSuite) TestUnitsCompleted(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	p0 := "postgres/0"
	p1 := "postgres/1"
	p2 := "postgres/2"

	resultSource := params.EntitiesResults{
		Results: []params.EntitiesResult{{Entities: []params.Entity{
			{Tag: p0},
			{Tag: p1},
			{Tag: p2},
		}}},
	}
	fCaller.EXPECT().FacadeCall("UnitsCompleted", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	units, err := api.UnitsCompleted()
	c.Assert(err, gc.IsNil)

	expected := []names.UnitTag{names.NewUnitTag(p0), names.NewUnitTag(p1), names.NewUnitTag(p2)}
	c.Check(units, jc.SameContents, expected)
}

func (s *upgradeSeriesSuite) TestStartUnitCompletion(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatus{
			{Entity: s.args.Entities[0]},
		},
	}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	fCaller.EXPECT().FacadeCall("StartUnitCompletion", args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	err := api.StartUnitCompletion()
	c.Assert(err, gc.IsNil)
}
