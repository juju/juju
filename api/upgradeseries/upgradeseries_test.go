// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
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
			Status: params.UpgradeSeriesStatus{Status: model.UpgradeSeriesPrepareStarted, Entity: s.args.Entities[0]}},
		},
	}
	fCaller.EXPECT().FacadeCall("MachineStatus", s.args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	status, err := api.MachineStatus()
	c.Assert(err, gc.IsNil)
	c.Check(status, gc.Equals, model.UpgradeSeriesPrepareStarted)
}

func (s *upgradeSeriesSuite) TestSetMachineStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	fCaller := mocks.NewMockFacadeCaller(ctrl)

	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatus{
			{Status: model.UpgradeSeriesCompleteStarted, Entity: s.args.Entities[0]},
		},
	}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	fCaller.EXPECT().FacadeCall("SetMachineStatus", args, gomock.Any()).SetArg(2, resultSource)

	api := upgradeseries.NewStateFromCaller(fCaller, s.tag)
	err := api.SetMachineStatus(model.UpgradeSeriesCompleteStarted)
	c.Assert(err, gc.IsNil)
}
