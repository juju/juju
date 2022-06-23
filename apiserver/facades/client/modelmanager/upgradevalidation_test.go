// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&upgradeValidationSuite{})

type upgradeValidationSuite struct {
	jujutesting.IsolationSuite
}

func (s *upgradeValidationSuite) TestModelUpgradeCheckFailEarly(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)
	state := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	gomock.InOrder(
		state.EXPECT().Model().Return(model, nil),
		model.EXPECT().Owner().Return(names.NewUserTag("admin")),
		model.EXPECT().Name().Return("model-1"),
	)

	checker := modelmanager.NewModelUpgradeCheck("", statePool, state,
		func(modelUUID string, pool modelmanager.StatePool, st modelmanager.State, model modelmanager.Model) (*modelmanager.Blocker, error) {
			return modelmanager.NewBlocker("model migration is in process"), nil
		},
		func(modelUUID string, pool modelmanager.StatePool, st modelmanager.State, model modelmanager.Model) (*modelmanager.Blocker, error) {
			return nil, errors.New("server is unreachable")
		},
	)

	blockers, err := checker.Validate()
	c.Assert(err, gc.ErrorMatches, `server is unreachable`)
	c.Assert(blockers, jc.ErrorIsNil)
}

func (s *upgradeValidationSuite) TestModelUpgradeCheck(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)
	state := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	gomock.InOrder(
		state.EXPECT().Model().Return(model, nil),
		model.EXPECT().Owner().Return(names.NewUserTag("admin")),
		model.EXPECT().Name().Return("model-1"),
	)

	checker := modelmanager.NewModelUpgradeCheck(coretesting.ModelTag.Id(), statePool, state,
		func(modelUUID string, pool modelmanager.StatePool, st modelmanager.State, model modelmanager.Model) (*modelmanager.Blocker, error) {
			return modelmanager.NewBlocker("model migration is in process"), nil
		},
		func(modelUUID string, pool modelmanager.StatePool, st modelmanager.State, model modelmanager.Model) (*modelmanager.Blocker, error) {
			return modelmanager.NewBlocker("unexpected upgrade series lock found"), nil
		},
	)

	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.ErrorMatches, `
model "admin/model-1":
	model migration is in process
	unexpected upgrade series lock found`[1:])
}

func (s *upgradeValidationSuite) TestCheckNoWinMachinesForModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := mocks.NewMockState(ctrl)
	gomock.InOrder(
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(0, nil),
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(1, nil),
	)

	blocker, err := modelmanager.CheckNoWinMachinesForModel("", nil, state, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = modelmanager.CheckNoWinMachinesForModel("", nil, state, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `model hosts 1 windows machine(s)`)
}
