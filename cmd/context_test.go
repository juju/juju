package cmd_test

import (
	"bytes"
	"errors"
	"io"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
)

func dummyContext(c *C) *cmd.Context {
	return &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}}
}

func str(stream io.Writer) string {
	return stream.(*bytes.Buffer).String()
}

type CtxCommand struct {
	Value   string
	Minimal bool
}

func (c *CtxCommand) Info() *cmd.Info {
	if c.Minimal {
		return &cmd.Info{"cmd-name", "", "", ""}
	}
	return &cmd.Info{"cmd-name", "<some arg>", "cmd-purpose", "cmd-doc"}
}

func (c *CtxCommand) Init(f *gnuflag.FlagSet, args []string) error {
	if !c.Minimal {
		f.StringVar(&c.Value, "opt", "", "opt-doc")
	}
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *CtxCommand) Run(ctx *cmd.Context) error {
	if c.Value == "error" {
		return errors.New("oh noes!")
	}
	ctx.Stdout.Write([]byte("hello stdout: " + c.Value))
	ctx.Stderr.Write([]byte("hello stderr: " + c.Value))
	return nil
}

var minUsage = "usage: cmd-name\n"
var fullUsage = `usage: cmd-name [options] <some arg>
purpose: cmd-purpose

options:
--opt (= "")
    opt-doc

cmd-doc
`

type ContextSuite struct{}

var _ = Suite(&ContextSuite{})

func AssertMainOutput(c *C, com cmd.Command, usage string) {
	ctx := dummyContext(c)
	result := cmd.Main(com, ctx, []string{"--unknown"})
	c.Assert(result, Equals, 2)
	c.Assert(str(ctx.Stdout), Equals, "")
	expected := "ERROR: flag provided but not defined: --unknown\n" + usage
	c.Assert(str(ctx.Stderr), Equals, expected)
}

func (s *CommandSuite) TestMainOutput(c *C) {
	AssertMainOutput(c, &CtxCommand{}, fullUsage)
	AssertMainOutput(c, &CtxCommand{Minimal: true}, minUsage)
}

func (s *CommandSuite) TestMainBadRun(c *C) {
	ctx := dummyContext(c)
	result := cmd.Main(&CtxCommand{}, ctx, []string{"--opt", "error"})
	c.Assert(result, Equals, 1)
	c.Assert(str(ctx.Stdout), Equals, "")
	c.Assert(str(ctx.Stderr), Equals, "ERROR: oh noes!\n")
}

func (s *CommandSuite) TestMainSuccess(c *C) {
	ctx := dummyContext(c)
	result := cmd.Main(&CtxCommand{}, ctx, []string{"--opt", "success!"})
	c.Assert(result, Equals, 0)
	c.Assert(str(ctx.Stdout), Equals, "hello stdout: success!")
	c.Assert(str(ctx.Stderr), Equals, "hello stderr: success!")
}

func (s *CommandSuite) TestHelp(c *C) {
	for _, arg := range []string{"-h", "--help"} {
		ctx := dummyContext(c)
		result := cmd.Main(&CtxCommand{}, ctx, []string{arg})
		c.Assert(result, Equals, 0)
		c.Assert(str(ctx.Stdout), Equals, "")
		c.Assert(str(ctx.Stderr), Equals, fullUsage)
	}
}
