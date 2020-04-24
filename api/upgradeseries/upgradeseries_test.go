// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/upgradeseries"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/testing"
)

type upgradeSeriesSuite struct {
	jujutesting.BaseSuite

	tag                                  names.Tag
	args                                 params.Entities
	upgradeSeriesStartUnitCompletionArgs params.UpgradeSeriesStartUnitCompletionParam
}

var _ = gc.Suite(&upgradeSeriesSuite{})

func (s *upgradeSeriesSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0")
	s.args = params.Entities{Entities: []params.Entity{{Tag: s.tag.String()}}}
	s.upgradeSeriesStartUnitCompletionArgs = params.UpgradeSeriesStartUnitCompletionParam{
		Entities: []params.Entity{{Tag: s.tag.String()}},
	}
	s.BaseSuite.SetUpTest(c)
}

func (s *upgradeSeriesSuite) TestMachineStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	resultSource := params.UpgradeSeriesStatusResults{
		Results: []params.UpgradeSeriesStatusResult{{
			Status: model.UpgradeSeriesPrepareStarted,
		}},
	}
	fCaller.EXPECT().FacadeCall("MachineStatus", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	status, err := api.MachineStatus()
	c.Assert(err, gc.IsNil)
	c.Check(status, gc.Equals, model.UpgradeSeriesPrepareStarted)
}

func (s *upgradeSeriesSuite) TestMachineStatusNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	resultSource := params.UpgradeSeriesStatusResults{
		Results: []params.UpgradeSeriesStatusResult{{
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
		Params: []params.UpgradeSeriesStatusParam{
			{Status: model.UpgradeSeriesCompleteStarted, Entity: s.args.Entities[0]},
		},
	}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	fCaller.EXPECT().FacadeCall("SetMachineStatus", args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	err := api.SetMachineStatus(model.UpgradeSeriesCompleteStarted, "")
	c.Assert(err, gc.IsNil)
}

func (s *upgradeSeriesSuite) TestTargetSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	resultSource := params.StringResults{
		Results: []params.StringResult{{
			Result: "bionic",
		}},
	}
	fCaller.EXPECT().FacadeCall("TargetSeries", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	target, err := api.TargetSeries()
	c.Assert(err, gc.IsNil)
	c.Check(target, gc.Equals, "bionic")
}

func (s *upgradeSeriesSuite) TestUnitsPrepared(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	r0 := names.NewUnitTag("redis/0")
	r1 := names.NewUnitTag("redis/1")

	resultSource := params.EntitiesResults{
		Results: []params.EntitiesResult{{Entities: []params.Entity{
			{Tag: r0.String()},
			{Tag: r1.String()},
		}}},
	}
	fCaller.EXPECT().FacadeCall("UnitsPrepared", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	units, err := api.UnitsPrepared()
	c.Assert(err, gc.IsNil)

	expected := []names.UnitTag{r0, r1}
	c.Check(units, jc.SameContents, expected)
}

func (s *upgradeSeriesSuite) TestUnitsCompleted(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	p0 := names.NewUnitTag("postgres/0")
	p1 := names.NewUnitTag("postgres/1")
	p2 := names.NewUnitTag("postgres/2")

	resultSource := params.EntitiesResults{
		Results: []params.EntitiesResult{{Entities: []params.Entity{
			{Tag: p0.String()},
			{Tag: p1.String()},
			{Tag: p2.String()},
		}}},
	}
	fCaller.EXPECT().FacadeCall("UnitsCompleted", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	units, err := api.UnitsCompleted()
	c.Assert(err, gc.IsNil)

	expected := []names.UnitTag{p0, p1, p2}
	c.Check(units, jc.SameContents, expected)
}

func (s *upgradeSeriesSuite) TestStartUnitCompletion(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	fCaller.EXPECT().FacadeCall("StartUnitCompletion", s.upgradeSeriesStartUnitCompletionArgs, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	err := api.StartUnitCompletion("")
	c.Assert(err, gc.IsNil)
}

func (s *upgradeSeriesSuite) TestFinishUpgradeSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{
			{Series: "xenial", Entity: s.args.Entities[0]},
		},
	}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	fCaller.EXPECT().FacadeCall("FinishUpgradeSeries", args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	err := api.FinishUpgradeSeries("xenial")
	c.Assert(err, gc.IsNil)
}
