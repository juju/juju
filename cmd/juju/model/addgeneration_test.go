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

type AddGenerationSuite struct {
	generationBaseSuite
}

var _ = gc.Suite(&AddGenerationSuite{})

func (s *AddGenerationSuite) runInit(args ...string) error {
	cmd := model.NewAddGenerationCommandForTest(nil, s.store)
	return cmdtesting.InitCommand(cmd, args)
}

func (s *AddGenerationSuite) TestInit(c *gc.C) {
	err := s.runInit()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AddGenerationSuite) TestInitFail(c *gc.C) {
	err := s.runInit("test")
	c.Assert(err, gc.ErrorMatches, "No arguments allowed")
}

func (s *AddGenerationSuite) runCommand(c *gc.C, api model.AddGenerationCommandAPI) (*cmd.Context, error) {
	cmd := model.NewAddGenerationCommandForTest(api, s.store)
	return cmdtesting.RunCommand(c, cmd)
}

func setUpMocks(c *gc.C) (*gomock.Controller, *mocks.MockAddGenerationCommandAPI) {
	mockController := gomock.NewController(c)
	mockAddGenerationCommandAPI := mocks.NewMockAddGenerationCommandAPI(mockController)
	mockAddGenerationCommandAPI.EXPECT().Close()
	return mockController, mockAddGenerationCommandAPI
}

func (s *AddGenerationSuite) TestRunCommand(c *gc.C) {
	mockController, mockAddGenerationCommandAPI := setUpMocks(c)
	defer mockController.Finish()

	mockAddGenerationCommandAPI.EXPECT().AddGeneration(gomock.Any()).Return(nil)

	ctx, err := s.runCommand(c, mockAddGenerationCommandAPI)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "target generation set to next\n")

	// ensure the model's store has been updated to 'next'.
	details, err := s.store.ModelByName(s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationNext)
}

func (s *AddGenerationSuite) TestRunCommandFail(c *gc.C) {
	mockController, mockAddGenerationCommandAPI := setUpMocks(c)
	defer mockController.Finish()

	mockAddGenerationCommandAPI.EXPECT().AddGeneration(gomock.Any()).Return(errors.Errorf("failme")).Times(1)

	_, err := s.runCommand(c, mockAddGenerationCommandAPI)
	c.Assert(err, gc.ErrorMatches, "failme")
}
