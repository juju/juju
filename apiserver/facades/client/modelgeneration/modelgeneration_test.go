// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
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
	modelUUID string

	api *modelgeneration.API
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
	defer s.setupModelGenerationAPI(c, func(_ *gomock.Controller, _ *mocks.MockState, mockModel *mocks.MockModel) {
		mExp := mockModel.EXPECT()
		mExp.AddGeneration().Return(nil)
	}).Finish()

	result, err := s.api.AddGeneration(s.modelArg())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *modelGenerationSuite) TestHasNextGeneration(c *gc.C) {
	defer s.setupModelGenerationAPI(c, func(_ *gomock.Controller, _ *mocks.MockState, mockModel *mocks.MockModel) {
		mockModel.EXPECT().HasNextGeneration().Return(true, nil)
	}).Finish()

	result, err := s.api.HasNextGeneration(s.modelArg())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Check(result.Result, jc.IsTrue)
}

func (s *modelGenerationSuite) TestAdvanceGenerationErrorNoAutoComplete(c *gc.C) {
	arg := params.AdvanceGenerationArg{
		Model: params.Entity{Tag: names.NewModelTag(s.modelUUID).String()},
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/0").String()},
			{Tag: names.NewApplicationTag("ghost").String()},
			{Tag: names.NewMachineTag("7").String()},
		},
	}

	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, _ *mocks.MockState, mockModel *mocks.MockModel) {
		mockGeneration := mocks.NewMockGeneration(ctrl)
		gExp := mockGeneration.EXPECT()
		gExp.AssignAllUnits("ghost").Return(nil)
		gExp.AssignUnit("mysql/0").Return(nil)
		gExp.AutoComplete().Return(false, nil)
		gExp.Refresh().Return(nil).Times(3)

		mExp := mockModel.EXPECT()
		mExp.NextGeneration().Return(mockGeneration, nil)
	}).Finish()

	result, err := s.api.AdvanceGeneration(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.AdvanceResults.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: &params.Error{Message: "expected names.UnitTag or names.ApplicationTag, got names.MachineTag"}},
	})
	c.Check(result.CompleteResult, gc.DeepEquals, params.BoolResult{})
}

func (s *modelGenerationSuite) TestAdvanceGenerationSuccessAutoComplete(c *gc.C) {
	arg := params.AdvanceGenerationArg{
		Model: params.Entity{Tag: names.NewModelTag(s.modelUUID).String()},
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/0").String()},
			{Tag: names.NewApplicationTag("ghost").String()},
		},
	}

	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, _ *mocks.MockState, mockModel *mocks.MockModel) {
		mockGeneration := mocks.NewMockGeneration(ctrl)
		gExp := mockGeneration.EXPECT()
		gExp.AssignAllUnits("ghost").Return(nil)
		gExp.AssignUnit("mysql/0").Return(nil)
		gExp.AutoComplete().Return(true, nil)
		gExp.Refresh().Return(nil).Times(2)

		mExp := mockModel.EXPECT()
		mExp.NextGeneration().Return(mockGeneration, nil)
	}).Finish()

	result, err := s.api.AdvanceGeneration(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.AdvanceResults.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
	})
	c.Check(result.CompleteResult, gc.DeepEquals, params.BoolResult{Result: true})
}

func (s *modelGenerationSuite) TestCancelGeneration(c *gc.C) {
	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, _ *mocks.MockState, mockModel *mocks.MockModel) {
		mockGeneration := mocks.NewMockGeneration(ctrl)
		gExp := mockGeneration.EXPECT()
		gExp.MakeCurrent().Return(nil)

		mExp := mockModel.EXPECT()
		mExp.NextGeneration().Return(mockGeneration, nil)
	}).Finish()

	result, err := s.api.CancelGeneration(s.modelArg())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: nil})
}

func (s *modelGenerationSuite) TestCancelGenerationCanNotMakeCurrent(c *gc.C) {
	errMsg := "cannot cancel generation, there are units behind a generation: riak/0"

	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, _ *mocks.MockState, mockModel *mocks.MockModel) {
		mockGeneration := mocks.NewMockGeneration(ctrl)
		gExp := mockGeneration.EXPECT()
		gExp.MakeCurrent().Return(errors.New(errMsg))

		mExp := mockModel.EXPECT()
		mExp.NextGeneration().Return(mockGeneration, nil)
	}).Finish()

	result, err := s.api.CancelGeneration(s.modelArg())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: &params.Error{Message: errMsg}})
}

func (s *modelGenerationSuite) TestGenerationInfo(c *gc.C) {
	units := []string{"redis/0", "redis/1", "redis/2"}

	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, st *mocks.MockState, mod *mocks.MockModel) {
		gen := mocks.NewMockGeneration(ctrl)
		gen.EXPECT().AssignedUnits().Return(map[string][]string{"redis": units})
		gen.EXPECT().Created().Return(int64(666))

		mod.EXPECT().NextGeneration().Return(gen, nil)

		app := mocks.NewMockApplication(ctrl)
		app.EXPECT().CharmConfig(model.GenerationCurrent).Return(map[string]interface{}{
			"databases": 16,
			"password":  "current",
		}, nil)
		app.EXPECT().CharmConfig(model.GenerationNext).Return(map[string]interface{}{
			"databases": 16,
			"password":  "next",
		}, nil)

		st.EXPECT().Application("redis").Return(app, nil)

	}).Finish()

	result, err := s.api.GenerationInfo(s.modelArg())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	gen := result.Generation
	c.Assert(gen.Created, gc.Equals, int64(666))
	c.Assert(gen.Applications, gc.HasLen, 1)

	app := gen.Applications[0]
	c.Assert(app.ApplicationName, gc.Equals, "redis")
	c.Assert(app.Units, jc.SameContents, units)
	c.Assert(app.ConfigChanges, gc.DeepEquals, map[string]interface{}{"password": "next"})
}

type setupFunc func(*gomock.Controller, *mocks.MockState, *mocks.MockModel)

func (s *modelGenerationSuite) setupModelGenerationAPI(c *gc.C, fn setupFunc) *gomock.Controller {
	ctrl := gomock.NewController(c)

	mockState := mocks.NewMockState(ctrl)
	sExp := mockState.EXPECT()
	sExp.ControllerTag().Return(names.NewControllerTag(s.modelUUID))

	mockModel := mocks.NewMockModel(ctrl)

	mockAuthorizer := facademocks.NewMockAuthorizer(ctrl)
	aExp := mockAuthorizer.EXPECT()
	aExp.HasPermission(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	aExp.GetAuthTag().Return(names.NewUserTag("testing"))
	aExp.AuthClient().Return(true)

	fn(ctrl, mockState, mockModel)

	var err error
	s.api, err = modelgeneration.NewModelGenerationAPI(mockState, mockAuthorizer, mockModel)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *modelGenerationSuite) modelArg() params.Entity {
	return params.Entity{Tag: names.NewModelTag(s.modelUUID).String()}
}
