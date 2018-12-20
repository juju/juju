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

type AddGenerationSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

var _ = gc.Suite(&AddGenerationSuite{})

func (s *AddGenerationSuite) SetUpTest(c *gc.C) {
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

func setUpMocks(c *gc.C) (*gomock.Controller, *MockAddGenerationCommandAPI) {
	mockController := gomock.NewController(c)
	mockAddGenerationCommandAPI := NewMockAddGenerationCommandAPI(mockController)
	mockAddGenerationCommandAPI.EXPECT().Close().Times(1)
	return mockController, mockAddGenerationCommandAPI
}

func (s *AddGenerationSuite) TestRunCommand(c *gc.C) {
	mockController, mockAddGenerationCommandAPI := setUpMocks(c)
	defer mockController.Finish()

	mockAddGenerationCommandAPI.EXPECT().AddGeneration().Return(nil).Times(1)

	ctx, err := s.runCommand(c, mockAddGenerationCommandAPI)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "target generation set to next\n")
}

func (s *AddGenerationSuite) TestRunCommandFail(c *gc.C) {
	c.Skip("Until apiserver ModelGeneration.AddGeneration() implemented")

	mockController, mockAddGenerationCommandAPI := setUpMocks(c)
	defer mockController.Finish()

	mockAddGenerationCommandAPI.EXPECT().AddGeneration().Return(errors.Errorf("failme")).Times(1)

	ctx, err := s.runCommand(c, mockAddGenerationCommandAPI)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "failme")
}
