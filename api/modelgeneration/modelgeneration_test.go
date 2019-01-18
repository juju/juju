// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration_test

import (
	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/modelgeneration"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

type modelGenerationSuite struct {
	tag names.ModelTag
}

var _ = gc.Suite(&modelGenerationSuite{})

func (s *modelGenerationSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewModelTag("deadbeef-abcd-4fd2-967d-db9663db7bea")
}

func (s *modelGenerationSuite) TestAddGeneration(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resultSource := params.ErrorResult{}
	arg := params.Entity{Tag: s.tag.String()}

	caller := mocks.NewMockAPICallCloser(ctrl)
	cExp := caller.EXPECT()
	cExp.BestFacadeVersion(gomock.Any()).Return(0).AnyTimes()

	fCaller := mocks.NewMockFacadeCaller(ctrl)
	fExp := fCaller.EXPECT()
	fExp.RawAPICaller().Return(caller).AnyTimes()
	fExp.FacadeCall("AddGeneration", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(fCaller)
	err := api.AddGeneration(s.tag)
	c.Assert(err, gc.IsNil)
}

func (s *modelGenerationSuite) TestAdvanceGeneration(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resultsSource := params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
		},
	}
	arg := params.AdvanceGenerationArg{
		Model: params.Entity{Tag: s.tag.String()},
		Entities: []params.Entity{
			{Tag: "unit-mysql-0"},
			{Tag: "application-mysql"},
		},
	}

	caller := mocks.NewMockAPICallCloser(ctrl)
	cExp := caller.EXPECT()
	cExp.BestFacadeVersion(gomock.Any()).Return(0).AnyTimes()

	fCaller := mocks.NewMockFacadeCaller(ctrl)
	fExp := fCaller.EXPECT()
	fExp.RawAPICaller().Return(caller).AnyTimes()
	fExp.FacadeCall("AdvanceGeneration", arg, gomock.Any()).SetArg(2, resultsSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(fCaller)
	err := api.AdvanceGeneration(s.tag, []string{"mysql/0", "mysql"})
	c.Assert(err, gc.IsNil)
}

func (s *modelGenerationSuite) TestAdvanceGenerationError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := mocks.NewMockAPICallCloser(ctrl)
	cExp := caller.EXPECT()
	cExp.BestFacadeVersion(gomock.Any()).Return(0).AnyTimes()

	fCaller := mocks.NewMockFacadeCaller(ctrl)
	fExp := fCaller.EXPECT()
	fExp.RawAPICaller().Return(caller).AnyTimes()

	api := modelgeneration.NewStateFromCaller(fCaller)
	err := api.AdvanceGeneration(s.tag, []string{"mysql/0", "mysql", "machine-3"})
	c.Assert(err, gc.ErrorMatches, "Must be application or unit")
}

func (s *modelGenerationSuite) TestSwitchGeneration(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resultSource := params.ErrorResult{}
	arg := params.GenerationVersionArg{
		Model:   params.Entity{Tag: s.tag.String()},
		Version: model.GenerationNext,
	}

	caller := mocks.NewMockAPICallCloser(ctrl)
	cExp := caller.EXPECT()
	cExp.BestFacadeVersion(gomock.Any()).Return(0).AnyTimes()

	fCaller := mocks.NewMockFacadeCaller(ctrl)
	fExp := fCaller.EXPECT()
	fExp.RawAPICaller().Return(caller).AnyTimes()
	fExp.FacadeCall("SwitchGeneration", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(fCaller)
	err := api.SwitchGeneration(s.tag, "next")
	c.Assert(err, gc.IsNil)
}

func (s *modelGenerationSuite) TestSwitchGenerationError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := mocks.NewMockAPICallCloser(ctrl)
	cExp := caller.EXPECT()
	cExp.BestFacadeVersion(gomock.Any()).Return(0).AnyTimes()

	fCaller := mocks.NewMockFacadeCaller(ctrl)
	fExp := fCaller.EXPECT()
	fExp.RawAPICaller().Return(caller).AnyTimes()

	api := modelgeneration.NewStateFromCaller(fCaller)
	err := api.SwitchGeneration(s.tag, "summer")
	c.Assert(err, gc.ErrorMatches, "version must be 'next' or 'current'")
}
