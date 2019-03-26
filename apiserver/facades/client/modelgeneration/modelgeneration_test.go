// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/modelgeneration"
	"github.com/juju/juju/apiserver/facades/client/modelgeneration/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

type modelGenerationSuite struct {
	modelUUID     string
	newBranchName string
	apiUser       string

	api *modelgeneration.API
}

var _ = gc.Suite(&modelGenerationSuite{})

func (s *modelGenerationSuite) SetUpSuite(c *gc.C) {
	s.modelUUID = "deadbeef-abcd-4fd2-967d-db9663db7bea"
	s.newBranchName = "new-branch"
	s.apiUser = "test-user"
}

func (s *modelGenerationSuite) TearDownTest(c *gc.C) {
	s.api = nil
}

// TODO (hml) 17-jan-2019
// Add more explicit permissions tests once that requirement is ironed out.

func (s *modelGenerationSuite) TestAddGeneration(c *gc.C) {
	defer s.setupModelGenerationAPI(c, func(_ *gomock.Controller, _ *mocks.MockState, mod *mocks.MockModel) {
		mod.EXPECT().AddBranch(s.newBranchName, s.apiUser).Return(nil)
	}).Finish()

	result, err := s.api.AddBranch(s.newBranchArg())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *modelGenerationSuite) TestTrackBranchEntityTypeError(c *gc.C) {
	arg := params.BranchTrackArg{
		Model:      params.Entity{Tag: names.NewModelTag(s.modelUUID).String()},
		BranchName: s.newBranchName,
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/0").String()},
			{Tag: names.NewApplicationTag("ghost").String()},
			{Tag: names.NewMachineTag("7").String()},
		},
	}

	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, _ *mocks.MockState, mod *mocks.MockModel) {
		gen := mocks.NewMockGeneration(ctrl)
		gExp := gen.EXPECT()
		gExp.AssignAllUnits("ghost").Return(nil)
		gExp.AssignUnit("mysql/0").Return(nil)

		mod.EXPECT().Branch(s.newBranchName).Return(gen, nil)
	}).Finish()

	result, err := s.api.AdvanceGeneration(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: &params.Error{Message: "expected names.UnitTag or names.ApplicationTag, got names.MachineTag"}},
	})
}

func (s *modelGenerationSuite) TestTrackBranchSuccess(c *gc.C) {
	arg := params.BranchTrackArg{
		Model:      params.Entity{Tag: names.NewModelTag(s.modelUUID).String()},
		BranchName: s.newBranchName,
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/0").String()},
			{Tag: names.NewApplicationTag("ghost").String()},
		},
	}

	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, _ *mocks.MockState, mod *mocks.MockModel) {
		gen := mocks.NewMockGeneration(ctrl)
		gExp := gen.EXPECT()
		gExp.AssignAllUnits("ghost").Return(nil)
		gExp.AssignUnit("mysql/0").Return(nil)

		mod.EXPECT().Branch(s.newBranchName).Return(gen, nil)
	}).Finish()

	result, err := s.api.AdvanceGeneration(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
	})
}

func (s *modelGenerationSuite) TestCommitGeneration(c *gc.C) {
	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, _ *mocks.MockState, mod *mocks.MockModel) {
		gen := mocks.NewMockGeneration(ctrl)
		gen.EXPECT().Commit(s.apiUser).Return(3, nil)
		mod.EXPECT().Branch(s.newBranchName).Return(gen, nil)
	}).Finish()

	result, err := s.api.CommitBranch(s.newBranchArg())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.IntResult{Result: 3, Error: nil})
}

func (s *modelGenerationSuite) TestGenerationInfo(c *gc.C) {
	units := []string{"redis/0", "redis/1", "redis/2"}

	defer s.setupModelGenerationAPI(c, func(ctrl *gomock.Controller, st *mocks.MockState, mod *mocks.MockModel) {
		gen := mocks.NewMockGeneration(ctrl)
		gExp := gen.EXPECT()
		gExp.BranchName().Return(s.newBranchName)
		gExp.AssignedUnits().Return(map[string][]string{"redis": units})
		gExp.Created().Return(int64(666))
		gExp.CreatedBy().Return(s.apiUser)

		mod.EXPECT().Branch(s.newBranchName).Return(gen, nil)

		app := mocks.NewMockApplication(ctrl)
		app.EXPECT().CharmConfig(model.GenerationMaster).Return(map[string]interface{}{
			"databases": 16,
			"password":  "current",
		}, nil)
		app.EXPECT().CharmConfig(s.newBranchName).Return(map[string]interface{}{
			"databases": 16,
			"password":  "next",
		}, nil)

		st.EXPECT().Application("redis").Return(app, nil)

	}).Finish()

	result, err := s.api.BranchInfo(s.newBranchArg())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	gen := result.Generation
	c.Assert(gen.BranchName, gc.Equals, s.newBranchName)
	c.Assert(gen.Created, gc.Equals, int64(666))
	c.Assert(gen.CreatedBy, gc.Equals, s.apiUser)
	c.Assert(gen.Applications, gc.HasLen, 1)

	app := gen.Applications[0]
	c.Assert(app.ApplicationName, gc.Equals, "redis")
	c.Assert(app.Units, jc.SameContents, units)
	c.Assert(app.ConfigChanges, gc.DeepEquals, map[string]interface{}{"password": "next"})
}

func (s *modelGenerationSuite) TestHasActiveBranchTrue(c *gc.C) {
	defer s.setupModelGenerationAPI(c, func(_ *gomock.Controller, _ *mocks.MockState, mockModel *mocks.MockModel) {
		mockModel.EXPECT().Branch(s.newBranchName).Return(nil, nil)
	}).Finish()

	result, err := s.api.HasActiveBranch(s.newBranchArg())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Check(result.Result, jc.IsTrue)
}

func (s *modelGenerationSuite) TestHasActiveBranchFalse(c *gc.C) {
	defer s.setupModelGenerationAPI(c, func(_ *gomock.Controller, _ *mocks.MockState, mockModel *mocks.MockModel) {
		mockModel.EXPECT().Branch(s.newBranchName).Return(nil, errors.NotFoundf(s.newBranchName))
	}).Finish()

	result, err := s.api.HasActiveBranch(s.newBranchArg())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Check(result.Result, jc.IsFalse)
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
	aExp.GetAuthTag().Return(names.NewUserTag("test-user"))
	aExp.AuthClient().Return(true)

	fn(ctrl, mockState, mockModel)

	var err error
	s.api, err = modelgeneration.NewModelGenerationAPI(mockState, mockAuthorizer, mockModel)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *modelGenerationSuite) newBranchArg() params.BranchArg {
	return params.BranchArg{
		BranchName: s.newBranchName,
		Model:      params.Entity{Tag: names.NewModelTag(s.modelUUID).String()},
	}
}
