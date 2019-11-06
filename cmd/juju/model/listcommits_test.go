// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"github.com/pkg/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
	coremodel "github.com/juju/juju/core/model"
)

type commitsSuite struct {
	generationBaseSuite

	api *mocks.MockListCommitsCommandAPI
}

var _ = gc.Suite(&commitsSuite{})

func (s *commitsSuite) TestInitNoArg(c *gc.C) {
	err := s.runInit()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *commitsSuite) TestInitOneArg(c *gc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, gc.ErrorMatches, `expected no arguments, but got 1`)
}

func (s *commitsSuite) TestRunCommandNextGenExists(c *gc.C) {
	defer s.setup(c).Finish()

	result := []coremodel.GenerationCommit{
		{
			Created:      "0001-01-01 00:00:00Z",
			CreatedBy:    "test-user",
			CommitNumber: 1,
			BranchName:   "bla",
		},
		{
			Created:      "0001-02-02 00:00:00Z",
			CreatedBy:    "test-user",
			CommitNumber: 2,
			BranchName:   "test",
		},
	}
	s.api.EXPECT().ListCommits(gomock.Any()).Return(result, nil)

	ctx, err := s.runCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	fmt.Println(cmdtesting.Stdout(ctx))
}

func (s *commitsSuite) TestRunCommandAPIError(c *gc.C) {
	defer s.setup(c).Finish()

	s.api.EXPECT().ListCommits(gomock.Any()).Return(nil, errors.New("boom"))

	_, err := s.runCommand(c)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *commitsSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewListCommitsCommandForTest(nil, s.store), args)
}

func (s *commitsSuite) runCommand(c *gc.C) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewListCommitsCommandForTest(s.api, s.store))
}

func (s *commitsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.api = mocks.NewMockListCommitsCommandAPI(ctrl)
	s.api.EXPECT().Close()
	return ctrl
}
