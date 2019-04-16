// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration_test

import (
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/modelgeneration"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

type modelGenerationSuite struct {
	fCaller *mocks.MockFacadeCaller

	tag        names.ModelTag
	branchName string
}

var _ = gc.Suite(&modelGenerationSuite{})

func (s *modelGenerationSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewModelTag("deadbeef-abcd-4fd2-967d-db9663db7bea")
	s.branchName = "new-branch"
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
	arg := params.BranchArg{
		Model:      params.Entity{Tag: s.tag.String()},
		BranchName: s.branchName,
	}

	s.fCaller.EXPECT().FacadeCall("AddBranch", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	err := api.AddBranch(s.tag.Id(), s.branchName)
	c.Assert(err, gc.IsNil)
}

func (s *modelGenerationSuite) TestTrackBranchSuccess(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultsSource := params.ErrorResults{Results: []params.ErrorResult{
		{Error: nil},
		{Error: nil},
	}}
	arg := params.BranchTrackArg{
		Model:      params.Entity{Tag: s.tag.String()},
		BranchName: s.branchName,
		Entities: []params.Entity{
			{Tag: "unit-mysql-0"},
			{Tag: "application-mysql"},
		},
	}

	s.fCaller.EXPECT().FacadeCall("TrackBranch", arg, gomock.Any()).SetArg(2, resultsSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	err := api.TrackBranch(s.tag.Id(), s.branchName, []string{"mysql/0", "mysql"})
	c.Assert(err, gc.IsNil)
}

func (s *modelGenerationSuite) TestTrackBranchError(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	err := api.TrackBranch(s.tag.Id(), s.branchName, []string{"mysql/0", "mysql", "machine-3"})
	c.Assert(err, gc.ErrorMatches, `"machine-3" is not an application or a unit`)
}

func (s *modelGenerationSuite) TestCommitBranch(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.IntResult{Result: 2}
	arg := params.BranchArg{
		Model:      params.Entity{Tag: s.tag.String()},
		BranchName: s.branchName,
	}

	s.fCaller.EXPECT().FacadeCall("CommitBranch", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	newGenID, err := api.CommitBranch(s.tag.Id(), "new-branch")
	c.Assert(err, gc.IsNil)
	c.Check(newGenID, gc.Equals, 2)
}

func (s *modelGenerationSuite) TestHasActiveBranch(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.BoolResult{Result: true}
	arg := params.BranchArg{
		Model:      params.Entity{Tag: s.tag.String()},
		BranchName: s.branchName,
	}

	s.fCaller.EXPECT().FacadeCall("HasActiveBranch", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)
	has, err := api.HasActiveBranch(s.tag.Id(), s.branchName)
	c.Assert(err, gc.IsNil)
	c.Check(has, jc.IsTrue)
}

func (s *modelGenerationSuite) TestGenerationInfo(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	resultSource := params.GenerationResults{Generations: []params.Generation{{
		BranchName: "new-branch",
		Created:    time.Time{}.Unix(),
		CreatedBy:  "test-user",
		Applications: []params.GenerationApplication{
			{
				ApplicationName: "redis",
				UnitProgress:    "1/2",
				UnitsTracking:   []string{"redis/0"},
				UnitsPending:    []string{"redis/1"},
				ConfigChanges:   map[string]interface{}{"databases": 8},
			},
		},
	}}}
	arg := params.BranchInfoArgs{
		BranchNames: []string{s.branchName},
		Detailed:    true,
	}

	s.fCaller.EXPECT().FacadeCall("BranchInfo", arg, gomock.Any()).SetArg(2, resultSource).Return(nil)

	api := modelgeneration.NewStateFromCaller(s.fCaller)

	formatTime := func(t time.Time) string {
		return t.UTC().Format("2006-01-02 15:04:05")
	}

	apps, err := api.BranchInfo(s.tag.Id(), s.branchName, true, formatTime)
	c.Assert(err, gc.IsNil)
	c.Check(apps, jc.DeepEquals, map[string]model.Generation{
		s.branchName: {
			Created:   "0001-01-01 00:00:00",
			CreatedBy: "test-user",
			Applications: []model.GenerationApplication{{
				ApplicationName: "redis",
				UnitProgress:    "1/2",
				UnitDetail: &model.GenerationUnits{
					UnitsTracking: []string{"redis/0"},
					UnitsPending:  []string{"redis/1"},
				},
				ConfigChanges: map[string]interface{}{"databases": 8},
			}},
		},
	})
}
