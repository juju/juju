// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

var _ = tc.Suite(&CmdSuite{})
var _ = tc.Suite(&CmdHelpSuite{})
var _ = tc.Suite(&CmdDocumentationSuite{})

type CmdSuite struct {
	testing.LoggingCleanupSuite

	ctx *cmd.Context
}

func (s *CmdSuite) SetUpTest(c *tc.C) {
	s.LoggingCleanupSuite.SetUpTest(c)
	s.ctx = cmdtesting.Context(c)
	loggo.ReplaceDefaultWriter(cmd.NewWarningWriter(s.ctx.Stderr))
}

func (s *CmdSuite) TestContext(c *tc.C) {
	c.Check(s.ctx.Context, tc.DeepEquals, context.Background())
	c.Check(s.ctx.AbsPath("/foo/bar"), tc.Equals, "/foo/bar")
	c.Check(s.ctx.AbsPath("/foo/../bar"), tc.Equals, "/bar")
	c.Check(s.ctx.AbsPath("foo/bar"), tc.Equals, filepath.Join(s.ctx.Dir, "foo/bar"))
	homeDir := os.Getenv("HOME")
	c.Check(s.ctx.AbsPath("~/foo/bar"), tc.Equals, filepath.Join(homeDir, "foo/bar"))
}

func (s *CmdSuite) TestWith(c *tc.C) {
	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := s.ctx.With(cancelCtx)
	c.Assert(ctx.Context, tc.DeepEquals, cancelCtx)
}

func (s *CmdSuite) TestContextGetenv(c *tc.C) {
	s.ctx.Env = make(map[string]string)
	before := s.ctx.Getenv("foo")
	s.ctx.Env["foo"] = "bar"
	after := s.ctx.Getenv("foo")

	c.Check(before, tc.Equals, "")
	c.Check(after, tc.Equals, "bar")
}

func (s *CmdSuite) TestContextSetenv(c *tc.C) {
	before := s.ctx.Env["foo"]
	s.ctx.Setenv("foo", "bar")
	after := s.ctx.Env["foo"]

	c.Check(before, tc.Equals, "")
	c.Check(after, tc.Equals, "bar")
}

func (s *CmdSuite) TestInfo(c *tc.C) {
	minimal := &TestCommand{Name: "verb", Minimal: true}
	help := minimal.Info().Help(cmdtesting.NewFlagSet())
	c.Assert(string(help), tc.Equals, minimalHelp)

	full := &TestCommand{Name: "verb"}
	f := cmdtesting.NewFlagSet()
	var ignored string
	f.StringVar(&ignored, "option", "", "option-doc")
	help = full.Info().Help(f)
	c.Assert(string(help), tc.Equals, fmt.Sprintf(fullHelp, "flag", "Flag"))

	optionInfo := full.Info()
	optionInfo.Doc = ""
	f.FlagKnownAs = "option"
	help = optionInfo.Help(f)
	c.Assert(string(help), tc.Equals, optionHelp)
}

var initErrorTests = []struct {
	c    *TestCommand
	help string
}{
	{&TestCommand{Name: "verb"}, fmt.Sprintf(fullHelp, "flag", strings.Title("flag"))},
	{&TestCommand{Name: "verb", Minimal: true}, minimalHelp},
}

func (s *CmdSuite) TestMainInitError(c *tc.C) {
	expected := "ERROR flag provided but not defined: --unknown\n"
	for _, t := range initErrorTests {
		s.SetUpTest(c)
		s.assertOptionError(c, t.c, expected)
		s.TearDownTest(c)
	}
}

func (s *CmdSuite) assertOptionError(c *tc.C, command *TestCommand, expected string) {
	result := cmd.Main(command, s.ctx, []string{"--unknown"})
	c.Assert(result, tc.Equals, 2)
	c.Assert(bufferString(s.ctx.Stdout), tc.Equals, "")
	c.Assert(bufferString(s.ctx.Stderr), tc.Equals, expected)
}

func (s *CmdSuite) TestMainFlagsAKA(c *tc.C) {
	s.assertOptionError(c,
		&TestCommand{Name: "verb", FlagAKA: "option"},
		"ERROR option provided but not defined: --unknown\n")
}

func (s *CmdSuite) TestMainRunError(c *tc.C) {
	result := cmd.Main(&TestCommand{Name: "verb"}, s.ctx, []string{"--option", "error"})
	c.Assert(result, tc.Equals, 1)
	c.Assert(bufferString(s.ctx.Stdout), tc.Equals, "")
	c.Assert(bufferString(s.ctx.Stderr), tc.Equals, "ERROR BAM!\n")
}

func (s *CmdSuite) TestMainRunSilentError(c *tc.C) {
	result := cmd.Main(&TestCommand{Name: "verb"}, s.ctx, []string{"--option", "silent-error"})
	c.Assert(result, tc.Equals, 1)
	c.Assert(bufferString(s.ctx.Stdout), tc.Equals, "")
	c.Assert(bufferString(s.ctx.Stderr), tc.Equals, "")
}

func (s *CmdSuite) TestMainSuccess(c *tc.C) {
	result := cmd.Main(&TestCommand{Name: "verb"}, s.ctx, []string{"--option", "success!"})
	c.Assert(result, tc.Equals, 0)
	c.Assert(bufferString(s.ctx.Stdout), tc.Equals, "success!\n")
	c.Assert(bufferString(s.ctx.Stderr), tc.Equals, "")
}

func (s *CmdSuite) TestStdin(c *tc.C) {
	const phrase = "Do you, Juju?"
	s.ctx.Stdin = bytes.NewBuffer([]byte(phrase))
	result := cmd.Main(&TestCommand{Name: "verb"}, s.ctx, []string{"--option", "echo"})
	c.Assert(result, tc.Equals, 0)
	c.Assert(bufferString(s.ctx.Stdout), tc.Equals, phrase)
	c.Assert(bufferString(s.ctx.Stderr), tc.Equals, "")
}

func (s *CmdSuite) TestMainHelp(c *tc.C) {
	for _, arg := range []string{"-h", "--help"} {
		s.SetUpTest(c)
		result := cmd.Main(&TestCommand{Name: "verb"}, s.ctx, []string{arg})
		c.Assert(result, tc.Equals, 0)
		c.Assert(bufferString(s.ctx.Stdout), tc.Equals, fmt.Sprintf(fullHelp, "flag", "Flag"))
		c.Assert(bufferString(s.ctx.Stderr), tc.Equals, "")
		s.TearDownTest(c)
	}
}

func (s *CmdSuite) TestMainHelpFlagsAKA(c *tc.C) {
	for _, arg := range []string{"-h", "--help"} {
		s.SetUpTest(c)
		result := cmd.Main(&TestCommand{Name: "verb", FlagAKA: "option"}, s.ctx, []string{arg})
		c.Assert(result, tc.Equals, 0)
		c.Assert(bufferString(s.ctx.Stdout), tc.Equals, fmt.Sprintf(fullHelp, "option", "Option"))
		c.Assert(bufferString(s.ctx.Stderr), tc.Equals, "")
		s.TearDownTest(c)
	}
}

func (s *CmdSuite) TestDefaultContextReturnsErrorInDeletedDirectory(c *tc.C) {
	wd, err := os.Getwd()
	c.Assert(err, tc.IsNil)
	missing := s.ctx.Dir + "/missing"
	err = os.Mkdir(missing, 0700)
	c.Assert(err, tc.IsNil)
	err = os.Chdir(missing)
	c.Assert(err, tc.IsNil)
	defer os.Chdir(wd)
	err = os.Remove(missing)
	c.Assert(err, tc.IsNil)
	ctx, err := cmd.DefaultContext()
	c.Assert(err, tc.ErrorMatches, `getwd: no such file or directory`)
	c.Assert(ctx, tc.IsNil)
}

func (s *CmdSuite) TestCheckEmpty(c *tc.C) {
	c.Assert(cmd.CheckEmpty(nil), tc.IsNil)
	c.Assert(cmd.CheckEmpty([]string{"boo!"}), tc.ErrorMatches, `unrecognized args: \["boo!"\]`)
}

func (s *CmdSuite) TestZeroOrOneArgs(c *tc.C) {

	expectValue := func(args []string, expected string) {
		arg, err := cmd.ZeroOrOneArgs(args)
		c.Assert(arg, tc.Equals, expected)
		c.Assert(err, tc.IsNil)
	}

	expectValue(nil, "")
	expectValue([]string{}, "")
	expectValue([]string{"foo"}, "foo")

	arg, err := cmd.ZeroOrOneArgs([]string{"foo", "bar"})
	c.Assert(arg, tc.Equals, "")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *CmdSuite) TestIsErrSilent(c *tc.C) {
	c.Assert(cmd.IsErrSilent(cmd.ErrSilent), tc.Equals, true)
	c.Assert(cmd.IsErrSilent(utils.NewRcPassthroughError(99)), tc.Equals, true)
	c.Assert(cmd.IsErrSilent(fmt.Errorf("noisy")), tc.Equals, false)
}

func (s *CmdSuite) TestInfoHelp(c *tc.C) {
	fs := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	s.assertFlagSetHelp(c, fs)
}

func (s *CmdSuite) TestInfoHelpFlagsAKA(c *tc.C) {
	fs := gnuflag.NewFlagSetWithFlagKnownAs("", gnuflag.ContinueOnError, "item")
	s.assertFlagSetHelp(c, fs)
}

func (s *CmdSuite) assertFlagSetHelp(c *tc.C, fs *gnuflag.FlagSet) {
	// Test that white space is trimmed consistently from cmd.Info.Purpose
	// (Help Summary) and cmd.Info.Doc (Help Details)
	option := "option"
	fs.StringVar(&option, "option", "", "option-doc")

	table := []struct {
		summary, details string
	}{
		{`
			verb the juju`,
			`
			verb-doc`},
		{`verb the juju`, `verb-doc`},
		{`
			
			verb the juju`,
			`
			
			verb-doc`},
		{`verb the juju    `, `verb-doc

		 `},
	}
	want := fmt.Sprintf(fullHelp, fs.FlagKnownAs, strings.Title(fs.FlagKnownAs))
	for _, tv := range table {
		i := cmd.Info{
			Name:    "verb",
			Args:    "<something>",
			Purpose: tv.summary,
			Doc:     tv.details,
		}
		got := string(i.Help(fs))
		c.Check(got, tc.Equals, want)
	}
}

type CmdHelpSuite struct {
	testing.LoggingCleanupSuite

	superfs   *gnuflag.FlagSet
	commandfs *gnuflag.FlagSet

	info cmd.Info
}

func (s *CmdHelpSuite) SetUpTest(c *tc.C) {
	s.LoggingCleanupSuite.SetUpTest(c)

	addOptions := func(f *gnuflag.FlagSet, options []string) {
		for _, a := range options {
			option := a
			f.StringVar(&option, option, "", "option-doc")
		}
	}

	s.commandfs = gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	addOptions(s.commandfs, []string{"one", "five", "three"})

	s.superfs = gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	addOptions(s.superfs, []string{"blackpanther", "captainamerica", "spiderman"})

	s.info = cmd.Info{
		Name:    "verb",
		Args:    "<something>",
		Purpose: "command purpose",
		Doc:     "command details",
	}
}

func (s *CmdHelpSuite) assertHelp(c *tc.C, expected string) {
	got := string(s.info.HelpWithSuperFlags(s.superfs, s.commandfs))
	c.Check(got, tc.Equals, expected)
}

var noSuperOptions = `
Usage: verb [flags] <something>

Summary:
command purpose

Flags:
--five (= "")
    option-doc
--one (= "")
    option-doc
--three (= "")
    option-doc

Details:
command details
`[1:]

func (s *CmdHelpSuite) TestNoSuperOptionsWanted(c *tc.C) {
	got := string(s.info.Help(s.commandfs))
	c.Check(got, tc.Equals, noSuperOptions)

	s.assertHelp(c, noSuperOptions)
}

func (s *CmdHelpSuite) TestSuperDoesNotHaveDesiredOptions(c *tc.C) {
	s.info.ShowSuperFlags = []string{"wanted"}
	s.assertHelp(c, noSuperOptions)
}

func (s *CmdHelpSuite) TestSuperHasOneDesiredOption(c *tc.C) {
	s.info.ShowSuperFlags = []string{"captainamerica"}
	s.assertHelp(c, `
Usage: verb [flags] <something>

Summary:
command purpose

Global Flags:
--captainamerica (= "")
    option-doc

Command Flags:
--five (= "")
    option-doc
--one (= "")
    option-doc
--three (= "")
    option-doc

Details:
command details
`[1:])
}

func (s *CmdHelpSuite) TestSuperHasManyDesiredOptions(c *tc.C) {
	s.superfs.FlagKnownAs = "option"
	s.info.ShowSuperFlags = []string{"spiderman", "blackpanther"}
	s.assertHelp(c, `
Usage: verb [flags] <something>

Summary:
command purpose

Global Options:
--blackpanther (= "")
    option-doc
--spiderman (= "")
    option-doc

Command Flags:
--five (= "")
    option-doc
--one (= "")
    option-doc
--three (= "")
    option-doc

Details:
command details
`[1:])
}

func (s *CmdHelpSuite) TestSuperShowsSubcommands(c *tc.C) {
	s.info.Subcommands = map[string]string{
		"application": "Wait for an application to reach a specified state.",
		"machine":     "Wait for a machine to reach a specified state.",
		"model":       "Wait for a model to reach a specified state.",
		"unit":        "Wait for a unit to reach a specified state.",
	}

	s.assertHelp(c, `
Usage: verb [flags] <something>

Summary:
command purpose

Flags:
--five (= "")
    option-doc
--one (= "")
    option-doc
--three (= "")
    option-doc

Details:
command details

Subcommands:
    application - Wait for an application to reach a specified state.
    machine     - Wait for a machine to reach a specified state.
    model       - Wait for a model to reach a specified state.
    unit        - Wait for a unit to reach a specified state.
`[1:])
}

type CmdDocumentationSuite struct {
	testing.LoggingCleanupSuite
}
