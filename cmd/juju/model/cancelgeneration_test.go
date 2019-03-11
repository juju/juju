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

type cancelGenerationSuite struct {
	generationBaseSuite
}

var _ = gc.Suite(&cancelGenerationSuite{})

func (s *cancelGenerationSuite) runInit(args ...string) error {
	cmd := model.NewCancelGenerationCommandForTest(nil, s.store)
	return cmdtesting.InitCommand(cmd, args)
}

func (s *cancelGenerationSuite) TestInit(c *gc.C) {
	err := s.runInit()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cancelGenerationSuite) TestInitFail(c *gc.C) {
	err := s.runInit("test")
	c.Assert(err, gc.ErrorMatches, "No arguments allowed")
}

func (s *cancelGenerationSuite) runCommand(c *gc.C, api model.CancelGenerationCommandAPI) (*cmd.Context, error) {
	cmd := model.NewCancelGenerationCommandForTest(api, s.store)
	return cmdtesting.RunCommand(c, cmd)
}

func setUpCancelMocks(c *gc.C) (*gomock.Controller, *mocks.MockCancelGenerationCommandAPI) {
	mockController := gomock.NewController(c)
	mockCancelGenerationCommandAPI := mocks.NewMockCancelGenerationCommandAPI(mockController)
	mockCancelGenerationCommandAPI.EXPECT().Close()
	return mockController, mockCancelGenerationCommandAPI
}

func (s *cancelGenerationSuite) TestRunCommand(c *gc.C) {
	mockController, mockCancelGenerationCommandAPI := setUpCancelMocks(c)
	defer mockController.Finish()

	mockCancelGenerationCommandAPI.EXPECT().CancelGeneration(gomock.Any()).Return(nil)

	ctx, err := s.runCommand(c, mockCancelGenerationCommandAPI)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "remaining incomplete changes dropped and target generation set to current\n")

	// ensure the model's store has been updated to 'current'.
	details, err := s.store.ModelByName(s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationCurrent)
}

func (s *cancelGenerationSuite) TestRunCommandFail(c *gc.C) {
	mockController, mockCancelGenerationCommandAPI := setUpCancelMocks(c)
	defer mockController.Finish()

	mockCancelGenerationCommandAPI.EXPECT().CancelGeneration(gomock.Any()).Return(errors.Errorf("failme"))

	_, err := s.runCommand(c, mockCancelGenerationCommandAPI)
	c.Assert(err, gc.ErrorMatches, "failme")
}
