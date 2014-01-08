// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"bytes"
	"net/http"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type CmdSuite struct{}

var _ = gc.Suite(&CmdSuite{})

func (s *CmdSuite) TestHttpTransport(c *gc.C) {
	transport := http.DefaultTransport.(*http.Transport)
	c.Assert(transport.DisableKeepAlives, jc.IsTrue)
}

func (s *CmdSuite) TestContext(c *gc.C) {
	ctx := testing.Context(c)
	c.Assert(ctx.AbsPath("/foo/bar"), gc.Equals, "/foo/bar")
	c.Assert(ctx.AbsPath("foo/bar"), gc.Equals, filepath.Join(ctx.Dir, "foo/bar"))
}

func (s *CmdSuite) TestInfo(c *gc.C) {
	minimal := &TestCommand{Name: "verb", Minimal: true}
	help := minimal.Info().Help(testing.NewFlagSet())
	c.Assert(string(help), gc.Equals, minimalHelp)

	full := &TestCommand{Name: "verb"}
	f := testing.NewFlagSet()
	var ignored string
	f.StringVar(&ignored, "option", "", "option-doc")
	help = full.Info().Help(f)
	c.Assert(string(help), gc.Equals, fullHelp)

	optionInfo := full.Info()
	optionInfo.Doc = ""
	help = optionInfo.Help(f)
	c.Assert(string(help), gc.Equals, optionHelp)
}

var initErrorTests = []struct {
	c    *TestCommand
	help string
}{
	{&TestCommand{Name: "verb"}, fullHelp},
	{&TestCommand{Name: "verb", Minimal: true}, minimalHelp},
}

func (s *CmdSuite) TestMainInitError(c *gc.C) {
	for _, t := range initErrorTests {
		ctx := testing.Context(c)
		result := cmd.Main(t.c, ctx, []string{"--unknown"})
		c.Assert(result, gc.Equals, 2)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		expected := "error: flag provided but not defined: --unknown\n"
		c.Assert(bufferString(ctx.Stderr), gc.Equals, expected)
	}
}

func (s *CmdSuite) TestMainRunError(c *gc.C) {
	ctx := testing.Context(c)
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "error"})
	c.Assert(result, gc.Equals, 1)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "error: BAM!\n")
}

func (s *CmdSuite) TestMainRunSilentError(c *gc.C) {
	ctx := testing.Context(c)
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "silent-error"})
	c.Assert(result, gc.Equals, 1)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *CmdSuite) TestMainSuccess(c *gc.C) {
	ctx := testing.Context(c)
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "success!"})
	c.Assert(result, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "success!\n")
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *CmdSuite) TestStdin(c *gc.C) {
	const phrase = "Do you, Juju?"
	ctx := testing.Context(c)
	ctx.Stdin = bytes.NewBuffer([]byte(phrase))
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "echo"})
	c.Assert(result, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, phrase)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *CmdSuite) TestMainHelp(c *gc.C) {
	for _, arg := range []string{"-h", "--help"} {
		ctx := testing.Context(c)
		result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{arg})
		c.Assert(result, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, fullHelp)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	}
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
