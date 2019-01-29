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
	"github.com/juju/juju/feature"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type switchGenerationSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

var _ = gc.Suite(&switchGenerationSuite{})

func (s *switchGenerationSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.Generations)
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}
	err := s.store.UpdateModel("testing", "admin/mymodel", jujuclient.ModelDetails{
		ModelUUID:       testing.ModelTag.Id(),
		ModelType:       coremodel.IAAS,
		ModelGeneration: coremodel.GenerationCurrent,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.store.Models["testing"].CurrentModel = "admin/mymodel"
}

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
	mockController, mockSwitchGenerationCommandAPI := setUpSwitchMocks(c)
	defer mockController.Finish()

	mockSwitchGenerationCommandAPI.EXPECT().SwitchGeneration(gomock.Any(), "current").Return(nil)

	ctx, err := s.runCommand(c, mockSwitchGenerationCommandAPI, "current")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "target generation set to current\n")

	// ensure the model's store has been updated to 'current'.
	details, err := s.store.ModelByName(s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationCurrent)
}

func (s *switchGenerationSuite) TestRunCommandNext(c *gc.C) {
	mockController, mockSwitchGenerationCommandAPI := setUpSwitchMocks(c)
	defer mockController.Finish()

	mockSwitchGenerationCommandAPI.EXPECT().SwitchGeneration(gomock.Any(), "next").Return(nil)

	ctx, err := s.runCommand(c, mockSwitchGenerationCommandAPI, "next")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "target generation set to next\n")

	// ensure the model's store has been updated to 'next'.
	details, err := s.store.ModelByName(s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationNext)
}

func (s *switchGenerationSuite) TestRunCommandFail(c *gc.C) {
	mockController, mockSwitchGenerationCommandAPI := setUpSwitchMocks(c)
	defer mockController.Finish()

	mockSwitchGenerationCommandAPI.EXPECT().SwitchGeneration(gomock.Any(), "next").Return(errors.Errorf("failme"))

	_, err := s.runCommand(c, mockSwitchGenerationCommandAPI, "next")
	c.Assert(err, gc.ErrorMatches, "failme")
}
