package cmd_test

import (
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
	help := minimal.Info().Help(dummyFlagSet())
	c.Assert(string(help), Equals, minimalHelp)

	full := &TestCommand{Name: "verb"}
	f := dummyFlagSet()
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
		ctx := dummyContext(c)
		result := cmd.Main(t.c, ctx, []string{"--unknown"})
		c.Assert(result, Equals, 2)
		c.Assert(bufferString(ctx.Stdout), Equals, "")
		expected := t.help + "error: flag provided but not defined: --unknown\n"
		c.Assert(bufferString(ctx.Stderr), Equals, expected)
	}
}

func (s *CmdSuite) TestMainRunError(c *C) {
	ctx := dummyContext(c)
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "error"})
	c.Assert(result, Equals, 1)
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	c.Assert(bufferString(ctx.Stderr), Equals, "error: BAM!\n")
}

func (s *CmdSuite) TestMainSuccess(c *C) {
	ctx := dummyContext(c)
	result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{"--option", "success!"})
	c.Assert(result, Equals, 0)
	c.Assert(bufferString(ctx.Stdout), Equals, "success!\n")
	c.Assert(bufferString(ctx.Stderr), Equals, "")
}

func (s *CmdSuite) TestMainHelp(c *C) {
	for _, arg := range []string{"-h", "--help"} {
		ctx := dummyContext(c)
		result := cmd.Main(&TestCommand{Name: "verb"}, ctx, []string{arg})
		c.Assert(result, Equals, 0)
		c.Assert(bufferString(ctx.Stdout), Equals, "")
		c.Assert(bufferString(ctx.Stderr), Equals, fullHelp)
	}
}

func (s *CmdSuite) TestCheckEmpty(c *C) {
	c.Assert(cmd.CheckEmpty(nil), IsNil)
	c.Assert(cmd.CheckEmpty([]string{"boo!"}), ErrorMatches, `unrecognized args: \["boo!"\]`)
}
