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
)

type AddGenerationSuite struct {
	generationBaseSuite
}

var _ = gc.Suite(&AddGenerationSuite{})

func (s *AddGenerationSuite) TestInit(c *gc.C) {
	err := s.runInit(s.branchName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AddGenerationSuite) TestInitFail(c *gc.C) {
	err := s.runInit()
	c.Assert(err, gc.ErrorMatches, "must specify a branch name")
}

func (s *AddGenerationSuite) TestRunCommand(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	api.EXPECT().AddGeneration(gomock.Any(), s.branchName).Return(nil)

	ctx, err := s.runCommand(c, api)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `target generation set to "`+s.branchName+"\"\n")

	// Ensure the local store has "new-branch" as the target.
	details, err := s.store.ModelByName(
		s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, s.branchName)
}

func (s *AddGenerationSuite) TestRunCommandFail(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	api.EXPECT().AddGeneration(gomock.Any(), s.branchName).Return(errors.Errorf("fail"))

	_, err := s.runCommand(c, api)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *AddGenerationSuite) runInit(args ...string) error {
	return cmdtesting.InitCommand(model.NewAddGenerationCommandForTest(nil, s.store), args)
}

func (s *AddGenerationSuite) runCommand(c *gc.C, api model.AddGenerationCommandAPI) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewAddGenerationCommandForTest(api, s.store), s.branchName)
}

func setUpMocks(c *gc.C) (*gomock.Controller, *mocks.MockAddGenerationCommandAPI) {
	ctrl := gomock.NewController(c)
	api := mocks.NewMockAddGenerationCommandAPI(ctrl)
	api.EXPECT().Close()
	return ctrl, api
}
