// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"os"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
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
	if err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.Generations); err != nil {
		panic(err)
	}
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}
	err := s.store.UpdateModel("testing", "admin/mymodel", jujuclient.ModelDetails{
		ModelUUID: testing.ModelTag.Id(),
		ModelType: coremodel.IAAS,
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

func setUpSwitchMocks(c *gc.C) (*gomock.Controller, *MockSwitchGenerationCommandAPI) {
	mockController := gomock.NewController(c)
	mockSwitchGenerationCommandAPI := NewMockSwitchGenerationCommandAPI(mockController)
	mockSwitchGenerationCommandAPI.EXPECT().Close()
	return mockController, mockSwitchGenerationCommandAPI
}

func (s *switchGenerationSuite) TestRunCommandCurrent(c *gc.C) {
	mockController, mockSwitchGenerationCommandAPI := setUpSwitchMocks(c)
	defer mockController.Finish()

	mockSwitchGenerationCommandAPI.EXPECT().SwitchGeneration("current").Return(nil).Times(1)

	ctx, err := s.runCommand(c, mockSwitchGenerationCommandAPI, "current")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "changes dropped and target generation set to current\n")
}

func (s *switchGenerationSuite) TestRunCommandNext(c *gc.C) {
	mockController, mockSwitchGenerationCommandAPI := setUpSwitchMocks(c)
	defer mockController.Finish()

	mockSwitchGenerationCommandAPI.EXPECT().SwitchGeneration("next").Return(nil).Times(1)

	ctx, err := s.runCommand(c, mockSwitchGenerationCommandAPI, "next")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "changes dropped and target generation set to next\n")
}

func (s *switchGenerationSuite) TestRunCommandFail(c *gc.C) {
	c.Skip("Until apiserver ModelGeneration.SwitchGeneration() implemented")

	mockController, mockSwitchGenerationCommandAPI := setUpSwitchMocks(c)
	defer mockController.Finish()

	mockSwitchGenerationCommandAPI.EXPECT().SwitchGeneration("").Return(errors.Errorf("failme")).Times(1)

	ctx, err := s.runCommand(c, mockSwitchGenerationCommandAPI)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "failme")
}
