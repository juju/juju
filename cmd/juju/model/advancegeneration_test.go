// Copyright 2019 Canonical Ltd.
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
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type advanceGenerationSuite struct {
	generationBaseSuite
}

var _ = gc.Suite(&advanceGenerationSuite{})

func (s *advanceGenerationSuite) SetUpTest(c *gc.C) {
	s.generationBaseSuite.SetUpTest(c)

	// Update the local store to indicate we are on the "next" generation.
	c.Assert(s.store.UpdateModel("testing", "admin/mymodel", jujuclient.ModelDetails{
		ModelUUID:       testing.ModelTag.Id(),
		ModelType:       coremodel.IAAS,
		ModelGeneration: coremodel.GenerationNext,
	}), jc.ErrorIsNil)
}

func (s *advanceGenerationSuite) runInit(args ...string) error {
	cmd := model.NewAdvanceGenerationCommandForTest(nil, s.store)
	return cmdtesting.InitCommand(cmd, args)
}

func (s *advanceGenerationSuite) TestInitApplication(c *gc.C) {
	err := s.runInit("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *advanceGenerationSuite) TestInitUnit(c *gc.C) {
	err := s.runInit("ubuntu/0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *advanceGenerationSuite) TestInitEmpty(c *gc.C) {
	err := s.runInit()
	c.Assert(err, gc.ErrorMatches, `unit and/or application names\(s\) must be specified`)
}

func (s *advanceGenerationSuite) TestInitInvalid(c *gc.C) {
	err := s.runInit("test me")
	c.Assert(err, gc.ErrorMatches, `invalid application or unit name "test me"`)
}

func (s *advanceGenerationSuite) runCommand(c *gc.C, api model.AdvanceGenerationCommandAPI, args ...string) (*cmd.Context, error) {
	cmd := model.NewAdvanceGenerationCommandForTest(api, s.store)
	return cmdtesting.RunCommand(c, cmd, args...)
}

func setUpAdvanceMocks(c *gc.C) (*gomock.Controller, *mocks.MockAdvanceGenerationCommandAPI) {
	mockController := gomock.NewController(c)
	mockAdvanceGenerationCommandAPI := mocks.NewMockAdvanceGenerationCommandAPI(mockController)
	mockAdvanceGenerationCommandAPI.EXPECT().Close()
	return mockController, mockAdvanceGenerationCommandAPI
}

func (s *advanceGenerationSuite) TestRunCommandNotCompleted(c *gc.C) {
	mockController, api := setUpAdvanceMocks(c)
	defer mockController.Finish()

	api.EXPECT().AdvanceGeneration(gomock.Any(), []string{"ubuntu/0", "redis"}).Return(false, nil)

	_, err := s.runCommand(c, api, "ubuntu/0", "redis")
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the generation did not change in the local store.
	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationNext)
}

func (s *advanceGenerationSuite) TestRunCommandCompleted(c *gc.C) {
	mockController, api := setUpAdvanceMocks(c)
	defer mockController.Finish()

	api.EXPECT().AdvanceGeneration(gomock.Any(), []string{"ubuntu/0", "redis"}).Return(true, nil)

	ctx, err := s.runCommand(c, api, "ubuntu/0", "redis")
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the generation was changed to "current" in the local store.
	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationCurrent)

	c.Assert(cmdtesting.Stdout(ctx), gc.Equals,
		"generation automatically completed; target generation set to \"current\"\n")
}

func (s *advanceGenerationSuite) TestRunCommandFail(c *gc.C) {
	mockController, api := setUpAdvanceMocks(c)
	defer mockController.Finish()

	api.EXPECT().AdvanceGeneration(gomock.Any(), []string{"ubuntu/0"}).Return(false, errors.Errorf("failme"))

	_, err := s.runCommand(c, api, "ubuntu/0")
	c.Assert(err, gc.ErrorMatches, "failme")
}
