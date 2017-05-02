// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

type CmdSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&CmdSuite{})

func (s *CmdSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

var deployTests = []struct {
	about                 string
	args                  []string
	expectCharmOrBundle   string
	expectApplicationName string
	expectNumUnits        int
	expectError           string
	expectConfigFile      string
}{{
	about:               "simple init",
	args:                []string{"charm-name"},
	expectCharmOrBundle: "charm-name",
	expectNumUnits:      1,
}, {
	about:                 "charm and application name specified",
	args:                  []string{"charm-name", "application-name"},
	expectCharmOrBundle:   "charm-name",
	expectApplicationName: "application-name",
	expectNumUnits:        1,
}, {
	about:               "--num-units long form",
	args:                []string{"--num-units", "33", "charm-name"},
	expectNumUnits:      33,
	expectCharmOrBundle: "charm-name",
}, {
	about:               "--num-units short form",
	args:                []string{"-n", "104", "charm-name"},
	expectNumUnits:      104,
	expectCharmOrBundle: "charm-name",
}, {
	about:               "config specified",
	args:                []string{"--config", "testconfig.yaml", "charm-name"},
	expectCharmOrBundle: "charm-name",
	expectNumUnits:      1,
	expectConfigFile:    "testconfig.yaml",
}, {
	about:       "missing args",
	expectError: "no charm or bundle specified",
}, {
	about:       "bad unit count",
	args:        []string{"charm-name", "--num-units", "0"},
	expectError: "--num-units must be a positive integer",
}, {
	about:       "bad unit count (short form)",
	args:        []string{"charm-name", "-n", "0"},
	expectError: "--num-units must be a positive integer",
}}

func (s *CmdSuite) TestDeployCommandInit(c *gc.C) {
	for i, t := range deployTests {
		c.Logf("\ntest %d: %s", i, t.about)
		wrappedDeployCmd := NewDeployCommandForTest(nil, nil)
		wrappedDeployCmd.SetClientStore(jujuclient.NewMemStore())
		err := cmdtesting.InitCommand(wrappedDeployCmd, t.args)
		if t.expectError != "" {
			c.Assert(err, gc.ErrorMatches, t.expectError)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		deployCmd := modelcmd.InnerCommand(wrappedDeployCmd).(*DeployCommand)
		c.Assert(deployCmd.ApplicationName, gc.Equals, t.expectApplicationName)
		c.Assert(deployCmd.CharmOrBundle, gc.Equals, t.expectCharmOrBundle)
		c.Assert(deployCmd.NumUnits, gc.Equals, t.expectNumUnits)
		c.Assert(deployCmd.Config.Path, gc.Equals, t.expectConfigFile)
	}
}

func (*CmdSuite) TestExposeCommandInitWithMissingArgs(c *gc.C) {
	cmd := NewExposeCommand()
	cmd.SetClientStore(NewMockStore())
	err := cmdtesting.InitCommand(cmd, nil)
	c.Assert(err, gc.ErrorMatches, "no application name specified")

	// environment tested elsewhere
}

func (*CmdSuite) TestUnexposeCommandInitWithMissingArgs(c *gc.C) {
	cmd := NewUnexposeCommand()
	cmd.SetClientStore(NewMockStore())
	err := cmdtesting.InitCommand(cmd, nil)
	c.Assert(err, gc.ErrorMatches, "no application name specified")
}

func initRemoveUnitCommand(args ...string) (cmd.Command, error) {
	com := NewRemoveUnitCommand()
	return com, cmdtesting.InitCommand(com, args)
}

func (*CmdSuite) TestRemoveUnitCommandInitMissingArgs(c *gc.C) {
	cmd := NewRemoveUnitCommand()
	cmd.SetClientStore(NewMockStore())
	err := cmdtesting.InitCommand(cmd, nil)
	c.Assert(err, gc.ErrorMatches, "no units specified")
}

func (*CmdSuite) TestRemoveUnitCommandInitInvalidUnit(c *gc.C) {
	cmd := NewRemoveUnitCommand()
	cmd.SetClientStore(NewMockStore())
	err := cmdtesting.InitCommand(cmd, []string{"seven/nine"})
	c.Assert(err, gc.ErrorMatches, `invalid unit name "seven/nine"`)
}

func NewMockStore() *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	store.Models["foo"] = &jujuclient.ControllerModels{
		CurrentModel: "admin/bar",
		Models:       map[string]jujuclient.ModelDetails{"admin/bar": {}},
	}
	return store
}
