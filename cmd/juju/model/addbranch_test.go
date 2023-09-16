// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
	coremodel "github.com/juju/juju/core/model"
)

type addBranchSuite struct {
	generationBaseSuite
}

var _ = gc.Suite(&addBranchSuite{})

func (s *addBranchSuite) TestInit(c *gc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *addBranchSuite) TestInitNoName(c *gc.C) {
	err := s.runInit()
	c.Assert(err, gc.ErrorMatches, "expected a branch name")
}

func (s *addBranchSuite) TestInitInvalidName(c *gc.C) {
	err := s.runInit(coremodel.GenerationMaster)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *addBranchSuite) TestRunCommand(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	api.EXPECT().AddBranch(s.branchName).Return(nil)

	ctx, err := s.runCommand(c, api)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "Created branch \""+s.branchName+"\" and set active\n")

	// Ensure the local store has "new-branch" as the target.
	details, err := s.store.ModelByName(
		s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ActiveBranch, gc.Equals, s.branchName)
}

func (s *addBranchSuite) TestRunCommandFail(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	api.EXPECT().AddBranch(s.branchName).Return(errors.Errorf("fail"))

	_, err := s.runCommand(c, api)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *addBranchSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewAddBranchCommandForTest(nil, s.store), args)
}

func (s *addBranchSuite) runCommand(c *gc.C, api model.AddBranchCommandAPI) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewAddBranchCommandForTest(api, s.store), s.branchName)
}

func setUpMocks(c *gc.C) (*gomock.Controller, *mocks.MockAddBranchCommandAPI) {
	ctrl := gomock.NewController(c)
	api := mocks.NewMockAddBranchCommandAPI(ctrl)
	api.EXPECT().Close()
	return ctrl, api
}
