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

type switchGenerationSuite struct {
	generationBaseSuite
}

var _ = gc.Suite(&switchGenerationSuite{})

func (s *switchGenerationSuite) runInit(args ...string) error {
	cmd := model.NewSwitchGenerationCommandForTest(nil, s.store)
	return cmdtesting.InitCommand(cmd, args)
}

func (s *switchGenerationSuite) TestInitNext(c *gc.C) {
	err := s.runInit("next")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *switchGenerationSuite) TestInitCurrent(c *gc.C) {
	err := s.runInit("current")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *switchGenerationSuite) TestInitFail(c *gc.C) {
	err := s.runInit()
	c.Assert(err, gc.ErrorMatches, "Must specify 'current' or 'next'")
}

func (s *switchGenerationSuite) runCommand(c *gc.C, api model.SwitchGenerationCommandAPI, args ...string) (*cmd.Context, error) {
	cmd := model.NewSwitchGenerationCommandForTest(api, s.store)
	return cmdtesting.RunCommand(c, cmd, args...)
}

func setUpSwitchMocks(c *gc.C) (*gomock.Controller, *mocks.MockSwitchGenerationCommandAPI) {
	mockController := gomock.NewController(c)
	mockSwitchGenerationCommandAPI := mocks.NewMockSwitchGenerationCommandAPI(mockController)
	mockSwitchGenerationCommandAPI.EXPECT().Close()
	return mockController, mockSwitchGenerationCommandAPI
}

func (s *switchGenerationSuite) TestRunCommandCurrent(c *gc.C) {
	ctx, err := s.runCommand(c, nil, "current")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "target generation set to current\n")

	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationCurrent)
}

func (s *switchGenerationSuite) TestRunCommandNextGenExists(c *gc.C) {
	mockController, mockSwitchGenerationCommandAPI := setUpSwitchMocks(c)
	defer mockController.Finish()

	mockSwitchGenerationCommandAPI.EXPECT().HasNextGeneration(gomock.Any()).Return(true, nil)

	ctx, err := s.runCommand(c, mockSwitchGenerationCommandAPI, "next")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "target generation set to next\n")

	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationNext)
}

func (s *switchGenerationSuite) TestRunCommandNextNoGenError(c *gc.C) {
	mockController, mockSwitchGenerationCommandAPI := setUpSwitchMocks(c)
	defer mockController.Finish()

	mockSwitchGenerationCommandAPI.EXPECT().HasNextGeneration(gomock.Any()).Return(false, nil)

	_, err := s.runCommand(c, mockSwitchGenerationCommandAPI, "next")
	c.Assert(err, gc.ErrorMatches, "this model has no next generation")
}
