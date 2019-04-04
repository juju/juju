// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
	coremodel "github.com/juju/juju/core/model"
)

type checkoutSuite struct {
	generationBaseSuite
}

var _ = gc.Suite(&checkoutSuite{})

func (s *checkoutSuite) TestInit(c *gc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *checkoutSuite) TestInitFail(c *gc.C) {
	err := s.runInit()
	c.Assert(err, gc.ErrorMatches, "must specify a branch name to switch to")
}

func (s *checkoutSuite) TestRunCommandCurrent(c *gc.C) {
	ctx, err := s.runCommand(c, nil, coremodel.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "target generation set to \"master\"\n")

	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationMaster)
}

func (s *checkoutSuite) TestRunCommandNextGenExists(c *gc.C) {
	ctrl, api := setUpSwitchMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(gomock.Any(), s.branchName).Return(true, nil)

	ctx, err := s.runCommand(c, api, s.branchName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "target generation set to \"new-branch\"\n")

	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, s.branchName)
}

func (s *checkoutSuite) TestRunCommandNextNoGenError(c *gc.C) {
	ctrl, api := setUpSwitchMocks(c)
	defer ctrl.Finish()

	api.EXPECT().HasActiveBranch(gomock.Any(), s.branchName).Return(false, nil)

	_, err := s.runCommand(c, api, s.branchName)
	c.Assert(err, gc.ErrorMatches, `this model has no active branch "`+s.branchName+`"`)
}

func (s *checkoutSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewSwitchGenerationCommandForTest(nil, s.store), args)
}

func (s *checkoutSuite) runCommand(c *gc.C, api model.SwitchGenerationCommandAPI, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewSwitchGenerationCommandForTest(api, s.store), args...)
}

func setUpSwitchMocks(c *gc.C) (*gomock.Controller, *mocks.MockSwitchGenerationCommandAPI) {
	ctrl := gomock.NewController(c)
	api := mocks.NewMockSwitchGenerationCommandAPI(ctrl)
	api.EXPECT().Close()
	return ctrl, api
}
