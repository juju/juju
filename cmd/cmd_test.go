// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"bytes"
	"path/filepath"
	stdtesting "testing"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

func Test(t *stdtesting.T) { TestingT(t) }

type CmdSuite struct{}

var _ = Suite(&CmdSuite{})

func (s *CmdSuite) TestContext(c *C) {
	ctx := testing.Context(c)
	c.Assert(ctx.AbsPath("/foo/bar"), Equals, "/foo/bar")
	c.Assert(ctx.AbsPath("foo/bar"), Equals, filepath.Join(ctx.Dir, "foo/bar"))
}

func (s *CmdSuite) TestInfo(c *C) {
	minimal := &TestCommand{Name: "verb", Minimal: true}
	help := minimal.Info().Help(testing.NewFlagSet())
	c.Assert(string(help), Equals, minimalHelp)

	full := &TestCommand{Name: "verb"}
	f := testing.NewFlagSet()
	var ignored string
	f.StringVar(&ignored, "option", "", "option-doc")
	help = full.Info().Help(f)
	c.Assert(string(help), Equals, fullHelp)

	optionInfo := full.Info()
	optionInfo.Doc = ""
	help = optionInfo.Help(f)
	c.Assert(string(help), Equals, optionHelp)
}

var initErrorTests = []struct {
	c    *TestCommand
	help string
}{
	{&TestCommand{Name: "verb"}, fullHelp},
	{&TestCommand{Name: "verb", Minimal: true}, minimalHelp},
}

func (s *CmdSuite) TestMainInitError(c *C) {
	for _, t := range initErrorTests {
		ctx := testing.Context(c)
		result := cmd.Main(t.c, ctx, []string{"--unknown"})
		c.Assert(result, Equals, 2)
		c.Assert(bufferString(ctx.Stdout), Equals, "")
		expected := "error: flag provided but not defined: --unknown\n"
		c.Assert(bufferString(ctx.Stderr), Equals, expected)
	}
}

func (s *CmdSuite) TestMainRunError(c *C) {
	ctx := testing.Context(c)
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "error"})
	c.Assert(result, Equals, 1)
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	c.Assert(bufferString(ctx.Stderr), Equals, "error: BAM!\n")
}

func (s *CmdSuite) TestMainRunSilentError(c *C) {
	ctx := testing.Context(c)
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "silent-error"})
	c.Assert(result, Equals, 1)
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	c.Assert(bufferString(ctx.Stderr), Equals, "")
}

func (s *CmdSuite) TestMainSuccess(c *C) {
	ctx := testing.Context(c)
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "success!"})
	c.Assert(result, Equals, 0)
	c.Assert(bufferString(ctx.Stdout), Equals, "success!\n")
	c.Assert(bufferString(ctx.Stderr), Equals, "")
}

func (s *CmdSuite) TestStdin(c *C) {
	const phrase = "Do you, Juju?"
	ctx := testing.Context(c)
	ctx.Stdin = bytes.NewBuffer([]byte(phrase))
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "echo"})
	c.Assert(result, Equals, 0)
	c.Assert(bufferString(ctx.Stdout), Equals, phrase)
	c.Assert(bufferString(ctx.Stderr), Equals, "")
}

func (s *CmdSuite) TestMainHelp(c *C) {
	for _, arg := range []string{"-h", "--help"} {
		ctx := testing.Context(c)
		result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{arg})
		c.Assert(result, Equals, 0)
		c.Assert(bufferString(ctx.Stdout), Equals, fullHelp)
		c.Assert(bufferString(ctx.Stderr), Equals, "")
	}
}

func (s *CmdSuite) TestCheckEmpty(c *C) {
	c.Assert(cmd.CheckEmpty(nil), IsNil)
	c.Assert(cmd.CheckEmpty([]string{"boo!"}), ErrorMatches, `unrecognized args: \["boo!"\]`)
}

func (s *CmdSuite) TestZeroOrOneArgs(c *C) {

	expectValue := func(args []string, expected string) {
		arg, err := cmd.ZeroOrOneArgs(args)
		c.Assert(arg, Equals, expected)
		c.Assert(err, IsNil)
	}

	expectValue(nil, "")
	expectValue([]string{}, "")
	expectValue([]string{"foo"}, "foo")

	arg, err := cmd.ZeroOrOneArgs([]string{"foo", "bar"})
	c.Assert(arg, Equals, "")
	c.Assert(err, ErrorMatches, `unrecognized args: \["bar"\]`)
}
