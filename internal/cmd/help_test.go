// Copyright 2012-2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package cmd_test

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
)

type HelpCommandSuite struct {
	testhelpers.IsolationSuite
}

func TestHelpCommandSuite(t *stdtesting.T) { tc.Run(t, &HelpCommandSuite{}) }
func (s *HelpCommandSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	loggo.GetLogger("juju.cmd").SetLogLevel(loggo.DEBUG)
}

func (s *HelpCommandSuite) assertStdOutMatches(c *tc.C, ctx *cmd.Context, match string) {
	stripped := strings.Replace(cmdtesting.Stdout(ctx), "\n", "", -1)
	c.Assert(stripped, tc.Matches, match)
}

func (s *HelpCommandSuite) TestHelpOutput(c *tc.C) {
	for i, test := range []struct {
		message     string
		args        []string
		usagePrefix string
		helpMatch   string
		errMatch    string
	}{
		{
			message:   "no args shows help",
			helpMatch: "Usage: jujutest .*",
		}, {
			message:     "usage prefix with help command",
			args:        []string{"help"},
			usagePrefix: "juju",
			helpMatch:   "Usage: juju jujutest .*",
		}, {
			message:     "usage prefix with help flag",
			args:        []string{"--help"},
			usagePrefix: "juju",
			helpMatch:   "Usage: juju jujutest .*",
		}, {
			message:   "help arg usage",
			args:      []string{"blah", "--help"},
			helpMatch: "Usage: jujutest blah.*blah-doc.*",
		}, {
			message:     "usage prefix with help command",
			args:        []string{"help", "blah"},
			usagePrefix: "juju",
			helpMatch:   "Usage: juju jujutest blah .*",
		}, {
			message:     "usage prefix with help flag",
			args:        []string{"blah", "--help"},
			usagePrefix: "juju",
			helpMatch:   "Usage: juju jujutest blah .*",
		}, {
			message:  "too many args",
			args:     []string{"help", "blah", "blah"},
			errMatch: `extra arguments to command help: \["blah"\]`,
		}, {
			args: []string{"help", "commands"},
			helpMatch: "blah\\s+blah the juju" +
				"documentation\\s+Generate the documentation for all commands" +
				"help\\s+Show help on a command or other topic.",
		},
	} {
		supername := "jujutest"
		super := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: supername, UsagePrefix: test.usagePrefix})
		super.Register(&TestCommand{Name: "blah"})
		c.Logf("%d: %s, %q", i, test.message, strings.Join(append([]string{supername}, test.args...), " "))

		ctx, err := cmdtesting.RunCommand(c, super, test.args...)
		if test.errMatch == "" {
			c.Assert(err, tc.ErrorIsNil)
			s.assertStdOutMatches(c, ctx, test.helpMatch)

		} else {
			c.Assert(err, tc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *HelpCommandSuite) TestHelpBasics(c *tc.C) {
	super := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "jujutest"})
	super.Register(&TestCommand{Name: "blah"})
	super.AddHelpTopic("basics", "short", "long help basics")

	ctx, err := cmdtesting.RunCommand(c, super)
	c.Assert(err, tc.ErrorIsNil)
	s.assertStdOutMatches(c, ctx, "long help basics")
}

func (s *HelpCommandSuite) TestMultipleSuperCommands(c *tc.C) {
	level1 := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "level1"})
	level2 := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "level2", UsagePrefix: "level1"})
	level1.Register(level2)
	level3 := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "level3", UsagePrefix: "level1 level2"})
	level2.Register(level3)
	level3.Register(&TestCommand{Name: "blah"})

	ctx, err := cmdtesting.RunCommand(c, level1, "help", "level2", "level3", "blah")
	c.Assert(err, tc.ErrorIsNil)
	s.assertStdOutMatches(c, ctx, "Usage: level1 level2 level3 blah.*blah-doc.*")

	_, err = cmdtesting.RunCommand(c, level1, "help", "level2", "missing", "blah")
	c.Assert(err, tc.ErrorMatches, `subcommand "missing" not found`)
}

func (s *HelpCommandSuite) TestAlias(c *tc.C) {
	super := cmd.NewSuperCommand(cmd.SuperCommandParams{Name: "super"})
	super.Register(&TestCommand{Name: "blah", Aliases: []string{"alias"}})
	ctx := cmdtesting.Context(c)
	code := cmd.Main(super, ctx, []string{"help", "alias"})
	c.Assert(code, tc.Equals, 0)
	stripped := strings.Replace(bufferString(ctx.Stdout), "\n", "", -1)
	c.Assert(stripped, tc.Matches, "Usage: super blah .*Aliases: alias")
}

func (s *HelpCommandSuite) TestRegisterSuperAliasHelp(c *tc.C) {
	jc := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "jujutest",
	})
	sub := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "bar",
		UsagePrefix: "jujutest",
		Purpose:     "bar functions",
	})
	jc.Register(sub)
	sub.Register(&simple{name: "foo"})

	jc.RegisterSuperAlias("bar-foo", "bar", "foo", nil)

	for _, test := range []struct {
		args []string
	}{
		{
			args: []string{"bar", "foo", "--help"},
		}, {
			args: []string{"bar", "help", "foo"},
		}, {
			args: []string{"help", "bar-foo"},
		}, {
			args: []string{"bar-foo", "--help"},
		},
	} {
		c.Logf("args: %v", test.args)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jc, ctx, test.args)
		c.Check(code, tc.Equals, 0)
		help := "Usage: jujutest bar foo\n\nSummary:\nto be simple\n"
		c.Check(cmdtesting.Stdout(ctx), tc.Equals, help)
	}
}

func (s *HelpCommandSuite) TestNotifyHelp(c *tc.C) {
	var called [][]string
	super := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "super",
		NotifyHelp: func(args []string) {
			called = append(called, args)
		},
	})
	super.Register(&TestCommand{
		Name: "blah",
	})
	ctx := cmdtesting.Context(c)
	code := cmd.Main(super, ctx, []string{"help", "blah"})
	c.Assert(code, tc.Equals, 0)

	c.Assert(called, tc.DeepEquals, [][]string{{"blah"}})
}
