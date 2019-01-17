// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/juju/core/model"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/modelgeneration"
	"github.com/juju/juju/apiserver/facades/client/modelgeneration/mocks"
	"github.com/juju/juju/apiserver/params"
)

var _ = gc.Suite(&modelGenerationSuite{})

type modelGenerationSuite struct {
}

// TODO (hml) 17-jan-2019
// Add more explicit permissions tests once that requirement is ironed out.

func (s *modelGenerationSuite) TestAddGeneration(c *gc.C) {
	api, ctrl := s.setupModelGenerationAPI(c, func(_ *gomock.Controller, mockModel *mocks.MockGenerationModel) {
		mExp := mockModel.EXPECT()
		mExp.AddGeneration().Return(nil)
	})
	defer ctrl.Finish()

	result, err := api.AddGeneration(params.Entity{Tag: names.NewModelTag("deadbeef-abcd-4fd2-967d-db9663db7bea").String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *modelGenerationSuite) TestAdvanceGeneration(c *gc.C) {
	arg := params.AdvanceGenerationArg{
		Model: params.Entity{Tag: names.NewModelTag("deadbeef-abcd-4fd2-967d-db9663db7bea").String()},
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/0").String()},
			{Tag: names.NewApplicationTag("ghost").String()},
			{Tag: names.NewMachineTag("7").String()},
		},
	}

	api, ctrl := s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, mockModel *mocks.MockGenerationModel) {
		mockGeneration := mocks.NewMockGeneration(ctrl)
		gExp := mockGeneration.EXPECT()
		gExp.Active().Return(true)
		gExp.AssignAllUnits("ghost").Return(nil)
		gExp.AssignUnit("mysql/0").Return(nil)
		gExp.CanMakeCurrent().Return(true, nil)
		gExp.MakeCurrent().Return(nil)
		gExp.Refresh().Return(nil).Times(3)

		mExp := mockModel.EXPECT()
		mExp.NextGeneration().Return(mockGeneration, nil)
	})
	defer ctrl.Finish()

	result, err := api.AdvanceGeneration(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: &params.Error{Message: "expected names.UnitTag or names.ApplicationTag, got names.MachineTag"}},
	})
}

func (s *modelGenerationSuite) TestSwitchGenerationNext(c *gc.C) {
	arg := params.GenerationVersionArg{
		Model:   params.Entity{Tag: names.NewModelTag("deadbeef-abcd-4fd2-967d-db9663db7bea").String()},
		Version: model.GenerationNext,
	}

	api, ctrl := s.setupModelGenerationAPI(c, func(_ *gomock.Controller, mockModel *mocks.MockGenerationModel) {
		mExp := mockModel.EXPECT()
		mExp.SwitchGeneration(arg.Version).Return(nil)
	})
	defer ctrl.Finish()

	result, err := api.SwitchGeneration(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

type setupFunc func(*gomock.Controller, *mocks.MockGenerationModel)

func (s *modelGenerationSuite) setupModelGenerationAPI(c *gc.C, fn setupFunc) (*modelgeneration.ModelGenerationAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	mockState := mocks.NewMockModelGenerationState(ctrl)
	sExp := mockState.EXPECT()
	sExp.ControllerTag().Return(names.NewControllerTag("deadbeef-babe-4fd2-967d-db9663db7bea"))

	mockModel := mocks.NewMockGenerationModel(ctrl)

	mockAuthorizer := facademocks.NewMockAuthorizer(ctrl)
	aExp := mockAuthorizer.EXPECT()
	aExp.HasPermission(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	aExp.GetAuthTag().Return(names.NewUserTag("testing"))
	aExp.AuthClient().Return(true)

	fn(ctrl, mockModel)
	api, err := modelgeneration.NewModelGenerationAPI(mockState, mockAuthorizer, mockModel)
	c.Assert(err, jc.ErrorIsNil)

	return api, ctrl
}
