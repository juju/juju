// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
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
		deployCmd := modelcmd.InnerCommand(wrappedDeployCmd).(*DeployCommand)
		c.Assert(deployCmd.ApplicationName, gc.Equals, t.expectApplicationName)
		c.Assert(deployCmd.CharmOrBundle, gc.Equals, t.expectCharmOrBundle)
		c.Assert(deployCmd.NumUnits, gc.Equals, t.expectNumUnits)
		c.Assert(deployCmd.Config.Path, gc.Equals, t.expectConfigFile)
	}
}

func initExposeCommand(args ...string) (*exposeCommand, error) {
	com := &exposeCommand{}
	return com, cmdtesting.InitCommand(modelcmd.Wrap(com), args)
}

func (*CmdSuite) TestExposeCommandInit(c *gc.C) {
	// missing args
	_, err := initExposeCommand()
	c.Assert(err, gc.ErrorMatches, "no application name specified")

	// environment tested elsewhere
}

func initUnexposeCommand(args ...string) (*unexposeCommand, error) {
	com := &unexposeCommand{}
	return com, cmdtesting.InitCommand(modelcmd.Wrap(com), args)
}

func (*CmdSuite) TestUnexposeCommandInit(c *gc.C) {
	// missing args
	_, err := initUnexposeCommand()
	c.Assert(err, gc.ErrorMatches, "no application name specified")

	// environment tested elsewhere
}

func initRemoveUnitCommand(args ...string) (cmd.Command, error) {
	com := NewRemoveUnitCommand()
	return com, cmdtesting.InitCommand(com, args)
}

func (*CmdSuite) TestRemoveUnitCommandInit(c *gc.C) {
	// missing args
	_, err := initRemoveUnitCommand()
	c.Assert(err, gc.ErrorMatches, "no units specified")
	// not a unit
	_, err = initRemoveUnitCommand("seven/nine")
	c.Assert(err, gc.ErrorMatches, `invalid unit name "seven/nine"`)
}
