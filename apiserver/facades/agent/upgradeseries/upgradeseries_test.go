// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/facades/agent/upgradeseries"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type upgradeSeriesSuite struct {
	testing.BaseSuite

	backend *mocks.MockUpgradeSeriesBackend
	machine *mocks.MockUpgradeSeriesMachine

	entityArgs                           params.Entities
	upgradeSeriesStatusArgs              params.UpgradeSeriesStatusParams
	upgradeSeriesStartUnitCompletionArgs params.UpgradeSeriesStartUnitCompletionParam

	api *upgradeseries.API

	machineTag names.MachineTag
	unitTag    names.UnitTag
}

var _ = gc.Suite(&upgradeSeriesSuite{})

func (s *upgradeSeriesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.machineTag = names.NewMachineTag("0")
	s.unitTag = names.NewUnitTag("redis/0")

	s.entityArgs = params.Entities{Entities: []params.Entity{{Tag: s.machineTag.String()}}}
	s.upgradeSeriesStatusArgs = params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{
			{
				Entity: params.Entity{Tag: s.machineTag.String()},
			},
		},
	}
	s.upgradeSeriesStartUnitCompletionArgs = params.UpgradeSeriesStartUnitCompletionParam{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	}
}

func (s *upgradeSeriesSuite) TestMachineStatus(c *gc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesPrepareCompleted, nil)

	results, err := s.api.MachineStatus(s.entityArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UpgradeSeriesStatusResults{
		Results: []params.UpgradeSeriesStatusResult{{Status: model.UpgradeSeriesPrepareCompleted}},
	})
}

func (s *upgradeSeriesSuite) TestSetMachineStatus(c *gc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, gomock.Any()).Return(nil)

	entity := params.Entity{Tag: s.machineTag.String()}
	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{{Entity: entity, Status: model.UpgradeSeriesPrepareCompleted}},
	}

	results, err := s.api.SetMachineStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesTarget(c *gc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().UpgradeSeriesTarget().Return("bionic", nil)

	results, err := s.api.TargetSeries(s.entityArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{Result: "bionic"}},
	})
}

func (s *upgradeSeriesSuite) TestStartUnitCompletion(c *gc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().StartUpgradeSeriesUnitCompletion(gomock.Any()).Return(nil)

	results, err := s.api.StartUnitCompletion(s.upgradeSeriesStartUnitCompletionArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *upgradeSeriesSuite) TestUnitsPrepared(c *gc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().UpgradeSeriesUnitStatuses().Return(map[string]state.UpgradeSeriesUnitStatus{
		"redis/0": {Status: model.UpgradeSeriesPrepareCompleted},
		"redis/1": {Status: model.UpgradeSeriesPrepareStarted},
	}, nil)

	results, err := s.api.UnitsPrepared(s.entityArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.EntitiesResults{
		Results: []params.EntitiesResult{{Entities: []params.Entity{{Tag: s.unitTag.String()}}}},
	})
}

func (s *upgradeSeriesSuite) TestUnitsCompleted(c *gc.C) {
	defer s.arrangeTest(c).Finish()

	s.machine.EXPECT().UpgradeSeriesUnitStatuses().Return(map[string]state.UpgradeSeriesUnitStatus{
		"redis/0": {Status: model.UpgradeSeriesCompleted},
		"redis/1": {Status: model.UpgradeSeriesCompleteStarted},
	}, nil)

	results, err := s.api.UnitsCompleted(s.entityArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.EntitiesResults{
		Results: []params.EntitiesResult{{Entities: []params.Entity{{Tag: s.unitTag.String()}}}},
	})
}

func (s *upgradeSeriesSuite) TestFinishUpgradeSeriesUpgraded(c *gc.C) {
	defer s.arrangeTest(c).Finish()

	exp := s.machine.EXPECT()
	exp.Series().Return("trusty")
	exp.UpdateMachineSeries("xenial", true).Return(nil)
	exp.RemoveUpgradeSeriesLock().Return(nil)

	entity := params.Entity{Tag: s.machineTag.String()}
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{Entity: entity, Series: "xenial"}},
	}

	results, err := s.api.FinishUpgradeSeries(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *upgradeSeriesSuite) TestFinishUpgradeSeriesNotUpgraded(c *gc.C) {
	defer s.arrangeTest(c).Finish()

	exp := s.machine.EXPECT()
	exp.Series().Return("trusty")
	exp.RemoveUpgradeSeriesLock().Return(nil)

	entity := params.Entity{Tag: s.machineTag.String()}
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{Entity: entity, Series: "trusty"}},
	}

	results, err := s.api.FinishUpgradeSeries(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *upgradeSeriesSuite) arrangeTest(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{Tag: s.machineTag}

	s.backend = mocks.NewMockUpgradeSeriesBackend(ctrl)
	s.machine = mocks.NewMockUpgradeSeriesMachine(ctrl)

	s.backend.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)

	var err error
	s.api, err = upgradeseries.NewUpgradeSeriesAPI(s.backend, resources, authorizer, nil)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}
