// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
	coremodel "github.com/juju/juju/core/model"
)

type branchSuite struct {
	generationBaseSuite
}

var _ = gc.Suite(&branchSuite{})

func (s *branchSuite) TestInit(c *gc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *branchSuite) TestInitNone(c *gc.C) {
	err := s.runInit()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *branchSuite) TestInitFail(c *gc.C) {
	err := s.runInit("test", "me")
	c.Assert(err, gc.ErrorMatches, "must specify a branch name to switch to or leave blank")
}

func (s *branchSuite) TestRunCommandMaster(c *gc.C) {
	ctx, err := s.runCommand(c, nil, coremodel.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "Active branch set to \"master\"\n")

	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ActiveBranch, gc.Equals, coremodel.GenerationMaster)
}

func (s *branchSuite) TestRunCommandBranchExists(c *gc.C) {
	ctrl, api := setUpSwitchMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(s.branchName).Return(true, nil)

	ctx, err := s.runCommand(c, api, s.branchName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "Active branch set to \"new-branch\"\n")

	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ActiveBranch, gc.Equals, s.branchName)
}

func (s *branchSuite) TestRunCommandNoBranchError(c *gc.C) {
	ctrl, api := setUpSwitchMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(s.branchName).Return(false, nil)

	_, err := s.runCommand(c, api, s.branchName)
	c.Assert(err, gc.ErrorMatches, `this model has no active branch "`+s.branchName+`"`)
}

func (s *branchSuite) TestRunCommandActiveBranch(c *gc.C) {
	ctx, err := s.runCommand(c, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "Active branch is \"master\"\n")
}

func (s *branchSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewBranchCommandForTest(nil, s.store), args)
}

func (s *branchSuite) runCommand(c *gc.C, api model.BranchCommandAPI, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewBranchCommandForTest(api, s.store), args...)
}

func setUpSwitchMocks(c *gc.C) (*gomock.Controller, *mocks.MockBranchCommandAPI) {
	ctrl := gomock.NewController(c)
	api := mocks.NewMockBranchCommandAPI(ctrl)
	api.EXPECT().Close()
	return ctrl, api
}
