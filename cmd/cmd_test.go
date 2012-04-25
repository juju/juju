package cmd_test

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type CmdSuite struct{}

var _ = Suite(&CmdSuite{})

func (s *CmdSuite) TestContext(c *C) {
	ctx := dummyContext(c)
	c.Assert(ctx.AbsPath("/foo/bar"), Equals, "/foo/bar")
	c.Assert(ctx.AbsPath("foo/bar"), Equals, filepath.Join(ctx.Dir, "foo/bar"))
}

func (s *CmdSuite) TestInfo(c *C) {
	minimal := &TestCommand{Name: "verb", Minimal: true}
	buf := &bytes.Buffer{}
	f := dummyFlagSet()
	minimal.Info().PrintHelp(buf, f)
	c.Assert(str(buf), Equals, minimalHelp)

	full := &TestCommand{Name: "verb"}
	buf = &bytes.Buffer{}
	var ignored string
	f.StringVar(&ignored, "option", "", "option-doc")
	full.Info().PrintHelp(buf, f)
	c.Assert(str(buf), Equals, fullHelp)
}

func (s *CmdSuite) TestMainInitError(c *C) {
	for _, t := range []struct {
		c    *TestCommand
		help string
	}{
		{&TestCommand{Name: "verb"}, fullHelp},
		{&TestCommand{Name: "verb", Minimal: true}, minimalHelp},
	} {
		ctx := dummyContext(c)
		result := cmd.Main(t.c, ctx, []string{"--unknown"})
		c.Assert(result, Equals, 2)
		c.Assert(str(ctx.Stdout), Equals, "")
		expected := "ERROR: flag provided but not defined: --unknown\n" + t.help
		c.Assert(str(ctx.Stderr), Equals, expected)
	}
}

func (s *CmdSuite) TestMainRunError(c *C) {
	ctx := dummyContext(c)
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "error"})
	c.Assert(result, Equals, 1)
	c.Assert(str(ctx.Stdout), Equals, "")
	c.Assert(str(ctx.Stderr), Equals, "ERROR: BAM!\n")
}

func (s *CmdSuite) TestMainSuccess(c *C) {
	ctx := dummyContext(c)
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "success!"})
	c.Assert(result, Equals, 0)
	c.Assert(str(ctx.Stdout), Equals, "success!\n")
	c.Assert(str(ctx.Stderr), Equals, "")
}

func (s *CmdSuite) TestMainHelp(c *C) {
	for _, arg := range []string{"-h", "--help"} {
		ctx := dummyContext(c)
		result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{arg})
		c.Assert(result, Equals, 0)
		c.Assert(str(ctx.Stdout), Equals, "")
		c.Assert(str(ctx.Stderr), Equals, fullHelp)
	}
}

func (s *CmdSuite) TestCheckEmpty(c *C) {
	c.Assert(cmd.CheckEmpty(nil), IsNil)
	c.Assert(cmd.CheckEmpty([]string{"boo!"}), ErrorMatches, `unrecognised args: \[boo!\]`)
}
