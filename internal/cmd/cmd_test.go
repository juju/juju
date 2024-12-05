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
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

var _ = gc.Suite(&CmdSuite{})
var _ = gc.Suite(&CmdHelpSuite{})
var _ = gc.Suite(&CmdDocumentationSuite{})

type CmdSuite struct {
	testing.LoggingCleanupSuite

	ctx *cmd.Context
}

func (s *CmdSuite) SetUpTest(c *gc.C) {
	s.LoggingCleanupSuite.SetUpTest(c)
	s.ctx = cmdtesting.Context(c)
	loggo.ReplaceDefaultWriter(cmd.NewWarningWriter(s.ctx.Stderr))
}

func (s *CmdSuite) TestContext(c *gc.C) {
	c.Check(s.ctx.Context, jc.DeepEquals, context.Background())
	c.Check(s.ctx.AbsPath("/foo/bar"), gc.Equals, "/foo/bar")
	c.Check(s.ctx.AbsPath("/foo/../bar"), gc.Equals, "/bar")
	c.Check(s.ctx.AbsPath("foo/bar"), gc.Equals, filepath.Join(s.ctx.Dir, "foo/bar"))
	homeDir := os.Getenv("HOME")
	c.Check(s.ctx.AbsPath("~/foo/bar"), gc.Equals, filepath.Join(homeDir, "foo/bar"))
}

func (s *CmdSuite) TestWith(c *gc.C) {
	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := s.ctx.With(cancelCtx)
	c.Assert(ctx.Context, jc.DeepEquals, cancelCtx)
}

func (s *CmdSuite) TestContextGetenv(c *gc.C) {
	s.ctx.Env = make(map[string]string)
	before := s.ctx.Getenv("foo")
	s.ctx.Env["foo"] = "bar"
	after := s.ctx.Getenv("foo")

	c.Check(before, gc.Equals, "")
	c.Check(after, gc.Equals, "bar")
}

func (s *CmdSuite) TestContextSetenv(c *gc.C) {
	before := s.ctx.Env["foo"]
	s.ctx.Setenv("foo", "bar")
	after := s.ctx.Env["foo"]

	c.Check(before, gc.Equals, "")
	c.Check(after, gc.Equals, "bar")
}

func (s *CmdSuite) TestInfo(c *gc.C) {
	minimal := &TestCommand{Name: "verb", Minimal: true}
	help := minimal.Info().Help(cmdtesting.NewFlagSet())
	c.Assert(string(help), gc.Equals, minimalHelp)

	full := &TestCommand{Name: "verb"}
	f := cmdtesting.NewFlagSet()
	var ignored string
	f.StringVar(&ignored, "option", "", "option-doc")
	help = full.Info().Help(f)
	c.Assert(string(help), gc.Equals, fmt.Sprintf(fullHelp, "flag", "Flag"))

	optionInfo := full.Info()
	optionInfo.Doc = ""
	f.FlagKnownAs = "option"
	help = optionInfo.Help(f)
	c.Assert(string(help), gc.Equals, optionHelp)
}

var initErrorTests = []struct {
	c    *TestCommand
	help string
}{
	{&TestCommand{Name: "verb"}, fmt.Sprintf(fullHelp, "flag", strings.Title("flag"))},
	{&TestCommand{Name: "verb", Minimal: true}, minimalHelp},
}

func (s *CmdSuite) TestMainInitError(c *gc.C) {
	expected := "ERROR flag provided but not defined: --unknown\n"
	for _, t := range initErrorTests {
		s.SetUpTest(c)
		s.assertOptionError(c, t.c, expected)
		s.TearDownTest(c)
	}
}

func (s *CmdSuite) assertOptionError(c *gc.C, command *TestCommand, expected string) {
	result := cmd.Main(command, s.ctx, []string{"--unknown"})
	c.Assert(result, gc.Equals, 2)
	c.Assert(bufferString(s.ctx.Stdout), gc.Equals, "")
	c.Assert(bufferString(s.ctx.Stderr), gc.Equals, expected)
}

func (s *CmdSuite) TestMainFlagsAKA(c *gc.C) {
	s.assertOptionError(c,
		&TestCommand{Name: "verb", FlagAKA: "option"},
		"ERROR option provided but not defined: --unknown\n")
}

func (s *CmdSuite) TestMainRunError(c *gc.C) {
	result := cmd.Main(&TestCommand{Name: "verb"}, s.ctx, []string{"--option", "error"})
	c.Assert(result, gc.Equals, 1)
	c.Assert(bufferString(s.ctx.Stdout), gc.Equals, "")
	c.Assert(bufferString(s.ctx.Stderr), gc.Equals, "ERROR BAM!\n")
}

func (s *CmdSuite) TestMainRunSilentError(c *gc.C) {
	result := cmd.Main(&TestCommand{Name: "verb"}, s.ctx, []string{"--option", "silent-error"})
	c.Assert(result, gc.Equals, 1)
	c.Assert(bufferString(s.ctx.Stdout), gc.Equals, "")
	c.Assert(bufferString(s.ctx.Stderr), gc.Equals, "")
}

func (s *CmdSuite) TestMainSuccess(c *gc.C) {
	result := cmd.Main(&TestCommand{Name: "verb"}, s.ctx, []string{"--option", "success!"})
	c.Assert(result, gc.Equals, 0)
	c.Assert(bufferString(s.ctx.Stdout), gc.Equals, "success!\n")
	c.Assert(bufferString(s.ctx.Stderr), gc.Equals, "")
}

func (s *CmdSuite) TestStdin(c *gc.C) {
	const phrase = "Do you, Juju?"
	s.ctx.Stdin = bytes.NewBuffer([]byte(phrase))
	result := cmd.Main(&TestCommand{Name: "verb"}, s.ctx, []string{"--option", "echo"})
	c.Assert(result, gc.Equals, 0)
	c.Assert(bufferString(s.ctx.Stdout), gc.Equals, phrase)
	c.Assert(bufferString(s.ctx.Stderr), gc.Equals, "")
}

func (s *CmdSuite) TestMainHelp(c *gc.C) {
	for _, arg := range []string{"-h", "--help"} {
		s.SetUpTest(c)
		result := cmd.Main(&TestCommand{Name: "verb"}, s.ctx, []string{arg})
		c.Assert(result, gc.Equals, 0)
		c.Assert(bufferString(s.ctx.Stdout), gc.Equals, fmt.Sprintf(fullHelp, "flag", "Flag"))
		c.Assert(bufferString(s.ctx.Stderr), gc.Equals, "")
		s.TearDownTest(c)
	}
}

func (s *CmdSuite) TestMainHelpFlagsAKA(c *gc.C) {
	for _, arg := range []string{"-h", "--help"} {
		s.SetUpTest(c)
		result := cmd.Main(&TestCommand{Name: "verb", FlagAKA: "option"}, s.ctx, []string{arg})
		c.Assert(result, gc.Equals, 0)
		c.Assert(bufferString(s.ctx.Stdout), gc.Equals, fmt.Sprintf(fullHelp, "option", "Option"))
		c.Assert(bufferString(s.ctx.Stderr), gc.Equals, "")
		s.TearDownTest(c)
	}
}

func (s *CmdSuite) TestDefaultContextReturnsErrorInDeletedDirectory(c *gc.C) {
	wd, err := os.Getwd()
	c.Assert(err, gc.IsNil)
	missing := s.ctx.Dir + "/missing"
	err = os.Mkdir(missing, 0700)
	c.Assert(err, gc.IsNil)
	err = os.Chdir(missing)
	c.Assert(err, gc.IsNil)
	defer os.Chdir(wd)
	err = os.Remove(missing)
	c.Assert(err, gc.IsNil)
	ctx, err := cmd.DefaultContext()
	c.Assert(err, gc.ErrorMatches, `getwd: no such file or directory`)
	c.Assert(ctx, gc.IsNil)
}

func (s *CmdSuite) TestCheckEmpty(c *gc.C) {
	c.Assert(cmd.CheckEmpty(nil), gc.IsNil)
	c.Assert(cmd.CheckEmpty([]string{"boo!"}), gc.ErrorMatches, `unrecognized args: \["boo!"\]`)
}

func (s *CmdSuite) TestZeroOrOneArgs(c *gc.C) {

	expectValue := func(args []string, expected string) {
		arg, err := cmd.ZeroOrOneArgs(args)
		c.Assert(arg, gc.Equals, expected)
		c.Assert(err, gc.IsNil)
	}

	expectValue(nil, "")
	expectValue([]string{}, "")
	expectValue([]string{"foo"}, "foo")

	arg, err := cmd.ZeroOrOneArgs([]string{"foo", "bar"})
	c.Assert(arg, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *CmdSuite) TestIsErrSilent(c *gc.C) {
	c.Assert(cmd.IsErrSilent(cmd.ErrSilent), gc.Equals, true)
	c.Assert(cmd.IsErrSilent(utils.NewRcPassthroughError(99)), gc.Equals, true)
	c.Assert(cmd.IsErrSilent(fmt.Errorf("noisy")), gc.Equals, false)
}

func (s *CmdSuite) TestInfoHelp(c *gc.C) {
	fs := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	s.assertFlagSetHelp(c, fs)
}

func (s *CmdSuite) TestInfoHelpFlagsAKA(c *gc.C) {
	fs := gnuflag.NewFlagSetWithFlagKnownAs("", gnuflag.ContinueOnError, "item")
	s.assertFlagSetHelp(c, fs)
}

func (s *CmdSuite) assertFlagSetHelp(c *gc.C, fs *gnuflag.FlagSet) {
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
		c.Check(got, gc.Equals, want)
	}
}

type CmdHelpSuite struct {
	testing.LoggingCleanupSuite

	superfs   *gnuflag.FlagSet
	commandfs *gnuflag.FlagSet

	info cmd.Info
}

func (s *CmdHelpSuite) SetUpTest(c *gc.C) {
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

func (s *CmdHelpSuite) assertHelp(c *gc.C, expected string) {
	got := string(s.info.HelpWithSuperFlags(s.superfs, s.commandfs))
	c.Check(got, gc.Equals, expected)
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

func (s *CmdHelpSuite) TestNoSuperOptionsWanted(c *gc.C) {
	got := string(s.info.Help(s.commandfs))
	c.Check(got, gc.Equals, noSuperOptions)

	s.assertHelp(c, noSuperOptions)
}

func (s *CmdHelpSuite) TestSuperDoesNotHaveDesiredOptions(c *gc.C) {
	s.info.ShowSuperFlags = []string{"wanted"}
	s.assertHelp(c, noSuperOptions)
}

func (s *CmdHelpSuite) TestSuperHasOneDesiredOption(c *gc.C) {
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

func (s *CmdHelpSuite) TestSuperHasManyDesiredOptions(c *gc.C) {
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

func (s *CmdHelpSuite) TestSuperShowsSubcommands(c *gc.C) {
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

	targetCmd cmd.Command
}
