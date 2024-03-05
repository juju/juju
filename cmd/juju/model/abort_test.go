// Copyright 2019 Canonical Ltd.
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

type abortSuite struct {
	generationBaseSuite
}

var _ = gc.Suite(&abortSuite{})

func (s *abortSuite) TestInit(c *gc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *abortSuite) TestInitNoName(c *gc.C) {
	err := s.runInit()
	c.Assert(err, gc.ErrorMatches, "expected a branch name")
}

func (s *abortSuite) TestInitInvalidName(c *gc.C) {
	err := s.runInit(coremodel.GenerationMaster)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *abortSuite) TestRunCommand(c *gc.C) {
	ctrl, api := setUpAbortMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(s.branchName).Return(true, nil)
	api.EXPECT().AbortBranch(s.branchName).Return(nil)

	ctx, err := s.runCommand(c, api)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "Aborting all changes in \""+s.branchName+"\" and closing branch.\n"+
		"Active branch set to \"master\"\n")

	// Ensure the local store has "new-branch" as the target.
	details, err := s.store.ModelByName(
		s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ActiveBranch, gc.Equals, "master")
}

func (s *abortSuite) TestRunCommandFail(c *gc.C) {
	ctrl, api := setUpAbortMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(s.branchName).Return(true, nil)
	api.EXPECT().AbortBranch(s.branchName).Return(errors.Errorf("fail"))

	_, err := s.runCommand(c, api)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *abortSuite) TestRunCommandFailHasActiveBranch(c *gc.C) {
	ctrl, api := setUpAbortMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(s.branchName).Return(false, nil)

	_, err := s.runCommand(c, api)
	c.Assert(err, gc.ErrorMatches, "this model has no active branch \""+s.branchName+"\"")
}

func (s *abortSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewAbortCommandForTest(nil, s.store), args)
}

func (s *abortSuite) runCommand(c *gc.C, api model.AbortCommandAPI) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewAbortCommandForTest(api, s.store), s.branchName)
}

func setUpAbortMocks(c *gc.C) (*gomock.Controller, *mocks.MockAbortCommandAPI) {
	ctrl := gomock.NewController(c)
	api := mocks.NewMockAbortCommandAPI(ctrl)
	api.EXPECT().Close()
	return ctrl, api
}
