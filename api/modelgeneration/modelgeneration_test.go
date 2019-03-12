// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration_test

import (
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/pkg/errors"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/modelgeneration"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

type modelGenerationSuite struct {
	tag     names.ModelTag
	fCaller *mocks.MockFacadeCaller
}

var _ = gc.Suite(&modelGenerationSuite{})

func (s *modelGenerationSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewModelTag("deadbeef-abcd-4fd2-967d-db9663db7bea")
}

func (s *modelGenerationSuite) TearDownTest(c *gc.C) {
	s.fCaller = nil
}

func (s *modelGenerationSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	caller := mocks.NewMockAPICallCloser(ctrl)
	caller.EXPECT().BestFacadeVersion(gomock.Any()).Return(0).AnyTimes()

	s.fCaller = mocks.NewMockFacadeCaller(ctrl)
	s.fCaller.EXPECT().RawAPICaller().Return(caller).AnyTimes()

	return ctrl
}

func (s *modelGenerationSuite) TestAddGeneration(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.ErrorResult{}
	arg := params.Entity{Tag: s.tag.String()}

	s.fCaller.EXPECT().FacadeCall("AddGeneration", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	err := api.AddGeneration(s.tag.Id())
	c.Assert(err, gc.IsNil)
}

func (s *modelGenerationSuite) TestAdvanceGenerationNoAutoComplete(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultsSource := params.AdvanceGenerationResult{
		AdvanceResults: params.ErrorResults{Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
		}},
	}
	arg := params.AdvanceGenerationArg{
		Model: params.Entity{Tag: s.tag.String()},
		Entities: []params.Entity{
			{Tag: "unit-mysql-0"},
			{Tag: "application-mysql"},
		},
	}

	s.fCaller.EXPECT().FacadeCall("AdvanceGeneration", arg, gomock.Any()).SetArg(2, resultsSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	completed, err := api.AdvanceGeneration(s.tag.Id(), []string{"mysql/0", "mysql"})
	c.Assert(err, gc.IsNil)
	c.Check(completed, jc.IsFalse)
}

func (s *modelGenerationSuite) TestAdvanceGenerationAdvanceError(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	completed, err := api.AdvanceGeneration(s.tag.Id(), []string{"mysql/0", "mysql", "machine-3"})
	c.Assert(err, gc.ErrorMatches, "Must be application or unit")
	c.Check(completed, jc.IsFalse)
}

func (s *modelGenerationSuite) TestAdvanceGenerationAutoComplete(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultsSource := params.AdvanceGenerationResult{
		AdvanceResults: params.ErrorResults{Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
		}},
		CompleteResult: params.BoolResult{Result: true},
	}
	arg := params.AdvanceGenerationArg{
		Model: params.Entity{Tag: s.tag.String()},
		Entities: []params.Entity{
			{Tag: "unit-mysql-0"},
			{Tag: "application-mysql"},
		},
	}

	s.fCaller.EXPECT().FacadeCall("AdvanceGeneration", arg, gomock.Any()).SetArg(2, resultsSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	completed, err := api.AdvanceGeneration(s.tag.Id(), []string{"mysql/0", "mysql"})
	c.Assert(err, gc.IsNil)
	c.Check(completed, jc.IsTrue)
}

func (s *modelGenerationSuite) TestAdvanceGenerationAutoCompleteError(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultsSource := params.AdvanceGenerationResult{
		AdvanceResults: params.ErrorResults{Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
		}},
		CompleteResult: params.BoolResult{Error: common.ServerError(errors.New("auto-complete go boom"))},
	}
	arg := params.AdvanceGenerationArg{
		Model: params.Entity{Tag: s.tag.String()},
		Entities: []params.Entity{
			{Tag: "unit-mysql-0"},
			{Tag: "application-mysql"},
		},
	}

	s.fCaller.EXPECT().FacadeCall("AdvanceGeneration", arg, gomock.Any()).SetArg(2, resultsSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	completed, err := api.AdvanceGeneration(s.tag.Id(), []string{"mysql/0", "mysql"})
	c.Assert(err, gc.ErrorMatches, "auto-complete go boom")
	c.Check(completed, jc.IsFalse)
}

func (s *modelGenerationSuite) TestCancelGeneration(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.ErrorResult{}
	arg := params.Entity{Tag: s.tag.String()}

	s.fCaller.EXPECT().FacadeCall("CancelGeneration", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	err := api.CancelGeneration(s.tag.Id())
	c.Assert(err, gc.IsNil)
}

func (s *modelGenerationSuite) TestHasNextGeneration(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.BoolResult{Result: true}
	arg := params.Entity{Tag: s.tag.String()}

	s.fCaller.EXPECT().FacadeCall("HasNextGeneration", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	has, err := api.HasNextGeneration(s.tag.Id())
	c.Assert(err, gc.IsNil)
	c.Check(has, jc.IsTrue)
}

func (s *modelGenerationSuite) TestGenerationInfo(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.GenerationResult{Generation: params.Generation{
		Created: time.Time{}.Unix(),
		Applications: []params.GenerationApplication{
			{
				ApplicationName: "redis",
				Units:           []string{"redis/0"},
				ConfigChanges:   map[string]interface{}{"databases": 8},
			},
		},
	}}
	arg := params.Entity{Tag: s.tag.String()}

	s.fCaller.EXPECT().FacadeCall("GenerationInfo", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)

	formatTime := func(t time.Time) string {
		return t.UTC().Format("2006-01-02 15:04:05")
	}

	apps, err := api.GenerationInfo(s.tag.Id(), formatTime)
	c.Assert(err, gc.IsNil)
	c.Check(apps, jc.DeepEquals, map[model.GenerationVersion]model.Generation{
		"next": {
			Created: "0001-01-01 00:00:00",
			Applications: []model.GenerationApplication{{
				ApplicationName: "redis",
				Units:           []string{"redis/0"},
				ConfigChanges:   map[string]interface{}{"databases": 8},
			}},
		},
	})
}
