// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
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

func (s *branchSuite) TestInitNoName(c *gc.C) {
	err := s.runInit()
	c.Assert(err, gc.ErrorMatches, "expected a branch name")
}

func (s *branchSuite) TestInitInvalidName(c *gc.C) {
	err := s.runInit(coremodel.GenerationMaster)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *branchSuite) TestRunCommand(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	api.EXPECT().AddBranch(s.branchName).Return(nil)

	ctx, err := s.runCommand(c, api)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `Active branch set to "`+s.branchName+"\"\n")

	// Ensure the local store has "new-branch" as the target.
	details, err := s.store.ModelByName(
		s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ActiveBranch, gc.Equals, s.branchName)
}

func (s *branchSuite) TestRunCommandFail(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	api.EXPECT().AddBranch(s.branchName).Return(errors.Errorf("fail"))

	_, err := s.runCommand(c, api)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *branchSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewBranchCommandForTest(nil, s.store), args)
}

func (s *branchSuite) runCommand(c *gc.C, api model.BranchCommandAPI) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewBranchCommandForTest(api, s.store), s.branchName)
}

func setUpMocks(c *gc.C) (*gomock.Controller, *mocks.MockBranchCommandAPI) {
	ctrl := gomock.NewController(c)
	api := mocks.NewMockBranchCommandAPI(ctrl)
	api.EXPECT().Close()
	return ctrl, api
}
