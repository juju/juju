// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
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
	cmd := model.NewSwitchGenerationCommandForTest(s.store)
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

func (s *switchGenerationSuite) runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := model.NewSwitchGenerationCommandForTest(s.store)
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *switchGenerationSuite) TestRunCommandCurrent(c *gc.C) {
	ctx, err := s.runCommand(c, "current")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "target generation set to current\n")

	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationCurrent)
}

func (s *switchGenerationSuite) TestRunCommandNext(c *gc.C) {
	ctx, err := s.runCommand(c, "next")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "target generation set to next\n")

	cName := s.store.CurrentControllerName
	details, err := s.store.ModelByName(cName, s.store.Models[cName].CurrentModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.ModelGeneration, gc.Equals, coremodel.GenerationNext)
}
