// Copyright 2019 Canonical Ltd.
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

type setGenerationSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

var _ = gc.Suite(&setGenerationSuite{})

func (s *setGenerationSuite) SetUpTest(c *gc.C) {
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

func (s *setGenerationSuite) runInit(args ...string) error {
	cmd := model.NewSetGenerationCommandForTest(nil, s.store)
	return cmdtesting.InitCommand(cmd, args)
}

func (s *setGenerationSuite) TestInitApplication(c *gc.C) {
	err := s.runInit("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *setGenerationSuite) TestInitUnit(c *gc.C) {
	err := s.runInit("ubuntu/0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *setGenerationSuite) TestInitEmpty(c *gc.C) {
	err := s.runInit()
	c.Assert(err, gc.ErrorMatches, `unit and/or application names\(s\) must be specified`)
}

func (s *setGenerationSuite) TestInitInvalid(c *gc.C) {
	err := s.runInit("test me")
	c.Assert(err, gc.ErrorMatches, `invalid application or unit name "test me"`)
}

func (s *setGenerationSuite) runCommand(c *gc.C, api model.SetGenerationCommandAPI, args ...string) (*cmd.Context, error) {
	cmd := model.NewSetGenerationCommandForTest(api, s.store)
	return cmdtesting.RunCommand(c, cmd, args...)
}

func setUpSetMocks(c *gc.C) (*gomock.Controller, *MockSetGenerationCommandAPI) {
	mockController := gomock.NewController(c)
	mockSetGenerationCommandAPI := NewMockSetGenerationCommandAPI(mockController)
	mockSetGenerationCommandAPI.EXPECT().Close().Times(1)
	return mockController, mockSetGenerationCommandAPI
}

func (s *setGenerationSuite) TestRunCommand(c *gc.C) {
	mockController, mockSetGenerationCommandAPI := setUpSetMocks(c)
	defer mockController.Finish()

	mockSetGenerationCommandAPI.EXPECT().SetGeneration([]string{"ubuntu/0", "redis"}).Return(nil)

	_, err := s.runCommand(c, mockSetGenerationCommandAPI, "ubuntu/0", "redis")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *setGenerationSuite) TestRunCommandFail(c *gc.C) {
	mockController, mockSetGenerationCommandAPI := setUpSetMocks(c)
	defer mockController.Finish()

	mockSetGenerationCommandAPI.EXPECT().SetGeneration([]string{"ubuntu/0"}).Return(errors.Errorf("failme"))

	_, err := s.runCommand(c, mockSetGenerationCommandAPI, "ubuntu/0")
	c.Assert(err, gc.ErrorMatches, "failme")
}
