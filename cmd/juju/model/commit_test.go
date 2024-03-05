// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
	coremodel "github.com/juju/juju/core/model"
)

type commitSuite struct {
	generationBaseSuite
}

var _ = gc.Suite(&commitSuite{})

func (s *commitSuite) TestInit(c *gc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *commitSuite) TestInitFail(c *gc.C) {
	err := s.runInit()
	c.Assert(err, gc.ErrorMatches, "must specify a branch name to commit")
}

func (s *commitSuite) TestRunCommandAborted(c *gc.C) {
	ctrl, api := setUpCancelMocks(c)
	defer ctrl.Finish()

	api.EXPECT().CommitBranch(s.branchName).Return(0, nil)

	ctx, err := s.runCommand(c, api)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Branch "new-branch" had no changes to commit and was aborted
Active branch set to "master"
`[1:])

	// Ensure the local store has "master" as the target.
	details, err := s.store.ModelByName(
		s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ActiveBranch, gc.Equals, coremodel.GenerationMaster)
}

func (s *commitSuite) TestRunCommandCommitted(c *gc.C) {
	ctrl, api := setUpCancelMocks(c)
	defer ctrl.Finish()

	api.EXPECT().CommitBranch(s.branchName).Return(3, nil)

	ctx, err := s.runCommand(c, api)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
Branch "new-branch" committed; model is now at generation 3
Active branch set to "master"
`[1:])

	// Ensure the local store has "master" as the target.
	details, err := s.store.ModelByName(
		s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ActiveBranch, gc.Equals, coremodel.GenerationMaster)
}

func (s *commitSuite) TestRunCommandFail(c *gc.C) {
	ctrl, api := setUpCancelMocks(c)
	defer ctrl.Finish()

	api.EXPECT().CommitBranch(s.branchName).Return(0, errors.Errorf("fail"))

	_, err := s.runCommand(c, api)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *commitSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewCommitCommandForTest(nil, s.store), args)
}

func (s *commitSuite) runCommand(c *gc.C, api model.CommitCommandAPI) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewCommitCommandForTest(api, s.store), s.branchName)
}

func setUpCancelMocks(c *gc.C) (*gomock.Controller, *mocks.MockCommitCommandAPI) {
	ctrl := gomock.NewController(c)
	api := mocks.NewMockCommitCommandAPI(ctrl)
	api.EXPECT().Close()
	return ctrl, api
}
