// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/cmd/juju/commands"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	coreversion "github.com/juju/juju/version"
)

type cmdUpgradeSuite struct {
	jujutesting.JujuConnSuite

	hostedModelUser    string
	hostedModelUserTag names.UserTag

	hostedModel string
}

func (s *cmdUpgradeSuite) SetUpTest(c *gc.C) {
	v, _ := version.Parse(oldVersion)
	s.PatchValue(&coreversion.Current, v)

	s.JujuConnSuite.SetUpTest(c)

	supported := series.SupportedLts()
	supported = append(supported, series.MustHostSeries())
	for _, aSeries := range supported {
		s.AddToolsToState(c, version.MustParseBinary(fmt.Sprintf("%v-%v-amd64", newVersion, aSeries)))
	}

	s.hostedModelUser = "otheruser"
	s.hostedModelUserTag = names.NewUserTag(s.hostedModelUser)

	s.hostedModel = "othermodel"
}

func (s *cmdUpgradeSuite) TestControllerAdminCanUpgradeHostedModel(c *gc.C) {
	testing.SkipIfWindowsBug(c, "lp:1446885")

	s.Factory.MakeUser(c, &factory.UserParams{Name: s.hostedModelUser})

	// Ensure we have hosted model.
	ctx := s.run(c, "add-model", s.hostedModel, "--owner", s.hostedModelUser)
	expectedModelAddedMsg := fmt.Sprintf("Added '%v' model on dummy/dummy-region for user '%v'", s.hostedModel, s.hostedModelUser)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, expectedModelAddedMsg)
	s.assertHostModelAgentVersion(c, oldVersion)

	// We are only testing here that controller admin can upgrade hosted model,
	// so it does not matter that the model is empty.
	// Upgrade hosted model.
	v, _ := version.Parse(newVersion)
	s.PatchValue(&coreversion.Current, v)
	ctx = s.run(c, "upgrade-model", "-m", fmt.Sprintf("%v/%v", s.hostedModelUser, s.hostedModel))
	expectedUpgradeMsg := fmt.Sprintf("started upgrade to %v", newVersion)
	c.Assert(cmdtesting.Stdout(ctx), jc.Contains, expectedUpgradeMsg)
	s.assertHostModelAgentVersion(c, newVersion)
}

var (
	oldVersion = "2.22.2"
	newVersion = "2.22.3"
)

func (s *cmdUpgradeSuite) run(c *gc.C, args ...string) *cmd.Context {
	context := cmdtesting.Context(c)
	jujuCmd := commands.NewJujuCommand(context, "")
	err := cmdtesting.InitCommand(jujuCmd, args)
	c.Assert(err, jc.ErrorIsNil)
	err = jujuCmd.Run(context)
	loggo.RemoveWriter("warning")
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdUpgradeSuite) assertHostModelAgentVersion(c *gc.C, desiredAgentVersion string) {
	modelUUIDs, err := s.State.ModelUUIDsForUser(s.hostedModelUserTag)
	c.Assert(err, jc.ErrorIsNil)

	var desiredModel *state.Model
	for _, modelUUID := range modelUUIDs {
		model, ph, err := s.StatePool.GetModel(modelUUID)
		c.Assert(err, jc.ErrorIsNil)
		defer ph.Release()
		if model.Name() == s.hostedModel {
			desiredModel = model
		}
	}
	c.Assert(desiredModel, gc.NotNil)

	cfg, err := desiredModel.Config()
	c.Assert(err, jc.ErrorIsNil)
	currentVersion, exists := cfg.AgentVersion()
	c.Assert(exists, jc.IsTrue)
	c.Assert(currentVersion.String(), gc.Equals, desiredAgentVersion)
}
