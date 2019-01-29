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
	"github.com/juju/juju/feature"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type advanceGenerationSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

var _ = gc.Suite(&advanceGenerationSuite{})

func (s *advanceGenerationSuite) SetUpTest(c *gc.C) {
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

func (s *advanceGenerationSuite) TestRunCommand(c *gc.C) {
	mockController, mockAdvanceGenerationCommandAPI := setUpAdvanceMocks(c)
	defer mockController.Finish()

	mockAdvanceGenerationCommandAPI.EXPECT().AdvanceGeneration(gomock.Any(), []string{"ubuntu/0", "redis"}).Return(nil)

	_, err := s.runCommand(c, mockAdvanceGenerationCommandAPI, "ubuntu/0", "redis")
	c.Assert(err, jc.ErrorIsNil)

	// ensure the model's store has been updated to 'current'.
	details, err := s.store.ModelByName(s.store.CurrentControllerName, s.store.Models[s.store.CurrentControllerName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationCurrent)
}

func (s *advanceGenerationSuite) TestRunCommandFail(c *gc.C) {
	mockController, mockAdvanceGenerationCommandAPI := setUpAdvanceMocks(c)
	defer mockController.Finish()

	mockAdvanceGenerationCommandAPI.EXPECT().AdvanceGeneration(gomock.Any(), []string{"ubuntu/0"}).Return(errors.Errorf("failme"))

	_, err := s.runCommand(c, mockAdvanceGenerationCommandAPI, "ubuntu/0")
	c.Assert(err, gc.ErrorMatches, "failme")
}
