// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type CmdSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

func TestCmdSuite(t *testing.T) {
	tc.Run(t, &CmdSuite{})
}

func (s *CmdSuite) SetUpTest(c *tc.C) {
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

func (s *CmdSuite) TestDeployCommandInit(c *tc.C) {
	for i, t := range deployTests {
		c.Logf("\ntest %d: %s", i, t.about)
		wrappedDeployCmd := newDeployCommandForTest(nil)
		err := cmdtesting.InitCommand(wrappedDeployCmd, t.args)
		if t.expectError != "" {
			c.Assert(err, tc.ErrorMatches, t.expectError)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		deployCmd := modelcmd.InnerCommand(wrappedDeployCmd).(*DeployCommand)
		c.Assert(deployCmd.ApplicationName, tc.Equals, t.expectApplicationName)
		c.Assert(deployCmd.CharmOrBundle, tc.Equals, t.expectCharmOrBundle)
		c.Assert(deployCmd.NumUnits, tc.Equals, t.expectNumUnits)
		if t.expectConfigFile != "" {
			ctx := cmdtesting.Context(c)
			absFiles, err := deployCmd.ConfigOptions.AbsoluteFileNames(ctx)
			c.Check(err, tc.ErrorIsNil)
			c.Check(absFiles, tc.HasLen, 1)
			c.Assert(absFiles[0], tc.Equals, filepath.Join(ctx.Dir, t.expectConfigFile))
		}
	}
}

func (*CmdSuite) TestExposeCommandInitWithMissingArgs(c *tc.C) {
	cmd := NewExposeCommand()
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	err := cmdtesting.InitCommand(cmd, nil)
	c.Assert(err, tc.ErrorMatches, "no application name specified")

	// environment tested elsewhere
}

func (*CmdSuite) TestUnexposeCommandInitWithMissingArgs(c *tc.C) {
	cmd := NewUnexposeCommand()
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	err := cmdtesting.InitCommand(cmd, nil)
	c.Assert(err, tc.ErrorMatches, "no application name specified")
}

func (*CmdSuite) TestRemoveUnitCommandInitMissingArgs(c *tc.C) {
	cmd := NewRemoveUnitCommand()
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	err := cmdtesting.InitCommand(cmd, nil)
	c.Assert(err, tc.ErrorMatches, "no units specified")
}

func (*CmdSuite) TestRemoveUnitCommandInitInvalidUnit(c *tc.C) {
	cmd := NewRemoveUnitCommand()
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	err := cmdtesting.InitCommand(cmd, []string{"seven/nine"})
	c.Assert(err, tc.ErrorMatches, `invalid unit name "seven/nine"`)
}
