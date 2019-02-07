// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration_test

import (
	"github.com/golang/mock/gomock"
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
	modelUUID string

	api *modelgeneration.ModelGenerationAPI
}

func (s *modelGenerationSuite) SetUpSuite(c *gc.C) {
	s.modelUUID = "deadbeef-abcd-4fd2-967d-db9663db7bea"
}

func (s *modelGenerationSuite) TearDownTest(c *gc.C) {
	s.api = nil
}

// TODO (hml) 17-jan-2019
// Add more explicit permissions tests once that requirement is ironed out.

func (s *modelGenerationSuite) TestAddGeneration(c *gc.C) {
	defer s.setupModelGenerationAPI(c, func(_ *gomock.Controller, mockModel *mocks.MockGenerationModel) {
		mExp := mockModel.EXPECT()
		mExp.AddGeneration().Return(nil)
	}).Finish()

	result, err := s.api.AddGeneration(params.Entity{Tag: names.NewModelTag(s.modelUUID).String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *modelGenerationSuite) TestHasNextGeneration(c *gc.C) {
	defer s.setupModelGenerationAPI(c, func(_ *gomock.Controller, mockModel *mocks.MockGenerationModel) {
		mockModel.EXPECT().HasNextGeneration().Return(true, nil)
	}).Finish()

	result, err := s.api.HasNextGeneration(params.Entity{Tag: names.NewModelTag(s.modelUUID).String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Check(result.Result, jc.IsTrue)
}

func (s *modelGenerationSuite) TestAdvanceGeneration(c *gc.C) {
	arg := params.AdvanceGenerationArg{
		Model: params.Entity{Tag: names.NewModelTag(s.modelUUID).String()},
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/0").String()},
			{Tag: names.NewApplicationTag("ghost").String()},
			{Tag: names.NewMachineTag("7").String()},
		},
	}

	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, mockModel *mocks.MockGenerationModel) {
		mockGeneration := mocks.NewMockGeneration(ctrl)
		gExp := mockGeneration.EXPECT()
		gExp.AssignAllUnits("ghost").Return(nil)
		gExp.AssignUnit("mysql/0").Return(nil)
		gExp.CanAutoComplete().Return(true, nil)
		gExp.AutoComplete().Return(nil)
		gExp.Refresh().Return(nil).Times(3)

		mExp := mockModel.EXPECT()
		mExp.NextGeneration().Return(mockGeneration, nil)
	}).Finish()

	result, err := s.api.AdvanceGeneration(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: &params.Error{Message: "expected names.UnitTag or names.ApplicationTag, got names.MachineTag"}},
	})
}

func (s *modelGenerationSuite) TestCancelGeneration(c *gc.C) {
	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, mockModel *mocks.MockGenerationModel) {
		mockGeneration := mocks.NewMockGeneration(ctrl)
		gExp := mockGeneration.EXPECT()
		gExp.CanMakeCurrent().Return(true, []string{}, nil)
		gExp.MakeCurrent().Return(nil)

		mExp := mockModel.EXPECT()
		mExp.NextGeneration().Return(mockGeneration, nil)
	}).Finish()

	result, err := s.api.CancelGeneration(params.Entity{Tag: names.NewModelTag(s.modelUUID).String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: nil})
}

func (s *modelGenerationSuite) TestCancelGenerationCanNotMakeCurrent(c *gc.C) {
	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, mockModel *mocks.MockGenerationModel) {
		mockGeneration := mocks.NewMockGeneration(ctrl)
		gExp := mockGeneration.EXPECT()
		gExp.CanMakeCurrent().Return(false, []string{"riak/0"}, nil)

		mExp := mockModel.EXPECT()
		mExp.NextGeneration().Return(mockGeneration, nil)
	}).Finish()

	result, err := s.api.CancelGeneration(params.Entity{Tag: names.NewModelTag(s.modelUUID).String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: &params.Error{Message: "cannot cancel generation, there are units behind a generation: riak/0"}})
}

type setupFunc func(*gomock.Controller, *mocks.MockGenerationModel)

func (s *modelGenerationSuite) setupModelGenerationAPI(c *gc.C, fn setupFunc) *gomock.Controller {
	ctrl := gomock.NewController(c)

	mockState := mocks.NewMockModelGenerationState(ctrl)
	sExp := mockState.EXPECT()
	sExp.ControllerTag().Return(names.NewControllerTag(s.modelUUID))

	mockModel := mocks.NewMockGenerationModel(ctrl)

	mockAuthorizer := facademocks.NewMockAuthorizer(ctrl)
	aExp := mockAuthorizer.EXPECT()
	aExp.HasPermission(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	aExp.GetAuthTag().Return(names.NewUserTag("testing"))
	aExp.AuthClient().Return(true)

	fn(ctrl, mockModel)

	var err error
	s.api, err = modelgeneration.NewModelGenerationAPI(mockState, mockAuthorizer, mockModel)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}
