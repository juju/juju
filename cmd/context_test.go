package cmd_test

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	cmd "launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/log"
	"path/filepath"
)

func getContext(c *C) *cmd.Context {
	return &cmd.Context{
		c.MkDir(), bytes.NewBuffer([]byte{}), bytes.NewBuffer([]byte{})}
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
		return &cmd.Info{"cmd-name", "", "", "", true}
	}
	return &cmd.Info{"cmd-name", "[options]", "cmd-purpose", "cmd-doc", true}
}

func (c *CtxCommand) InitFlagSet(f *gnuflag.FlagSet) {
	if !c.Minimal {
		f.StringVar(&c.Value, "opt", "", "opt-doc")
	}
}

func (c *CtxCommand) ParsePositional(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *CtxCommand) Run(ctx *cmd.Context) error {
	if c.Value == "error" {
		return errors.New("oh noes!")
	}
	ctx.Stdout.Write([]byte("hello stdout: " + c.Value))
	ctx.Stderr.Write([]byte("hello stderr: " + c.Value))
	return nil
}

var (
	fullUsage = `usage: cmd-name [options]
purpose: cmd-purpose

options:
--opt (= "")
    opt-doc

cmd-doc
`
	minUsage  = `usage: cmd-name
`
)

type ContextSuite struct{}

func AssertMainOutput(c *C, com cmd.Command, usage string) {
	ctx := getContext(c)
	result := ctx.Main(com, []string{"--unknown"})
	c.Assert(result, Equals, 2)
	c.Assert(str(ctx.Stdout), Equals, "")
	expected := "flag provided but not defined: --unknown\n" + usage
	c.Assert(str(ctx.Stderr), Equals, expected)
}

func (s *CommandSuite) TestMainOutput(c *C) {
	AssertMainOutput(c, &CtxCommand{}, fullUsage)
	AssertMainOutput(c, &CtxCommand{Minimal: true}, minUsage)
}

func (s *CommandSuite) TestMainBadRun(c *C) {
	ctx := getContext(c)
	result := ctx.Main(&CtxCommand{}, []string{"--opt", "error"})
	c.Assert(result, Equals, 1)
	c.Assert(str(ctx.Stdout), Equals, "")
	c.Assert(str(ctx.Stderr), Equals, "oh noes!\n")
}

func (s *CommandSuite) TestMainSuccess(c *C) {
	ctx := getContext(c)
	result := ctx.Main(&CtxCommand{}, []string{"--opt", "success!"})
	c.Assert(result, Equals, 0)
	c.Assert(str(ctx.Stdout), Equals, "hello stdout: success!")
	c.Assert(str(ctx.Stderr), Equals, "hello stderr: success!")
}

func AssertInitLog(c *C, verbose bool, debug bool, logfile string, logre string) {
	defer saveLog()()
	ctx := getContext(c)
	err := ctx.InitLog(verbose, debug, logfile)
	c.Assert(err, IsNil)
	log.Printf("hello log")
	log.Debugf("hello debug")
	c.Assert(str(ctx.Stdout), Equals, "")

	if logfile == "" {
		c.Assert(str(ctx.Stderr), Matches, logre)
	} else {
		c.Assert(str(ctx.Stderr), Equals, "")
		raw, err := ioutil.ReadFile(logfile)
		c.Assert(err, IsNil)
		c.Assert(string(raw), Matches, logre)
	}
}

func (s *CommandSuite) TestInitLog(c *C) {
	printfre := ".* JUJU hello log\n"
	debugfre := ".* JUJU:DEBUG hello debug\n"

	AssertInitLog(c, false, false, "", "")
	AssertInitLog(c, true, false, "", printfre)
	AssertInitLog(c, false, true, "", printfre+debugfre)
	AssertInitLog(c, true, true, "", printfre+debugfre)

	tmp := c.MkDir()
	AssertInitLog(c, false, false, filepath.Join(tmp, "1.log"), printfre)
	AssertInitLog(c, true, false, filepath.Join(tmp, "2.log"), printfre)
	AssertInitLog(c, false, true, filepath.Join(tmp, "3.log"), printfre+debugfre)
	AssertInitLog(c, true, true, filepath.Join(tmp, "4.log"), printfre+debugfre)
}

func (s *CommandSuite) TestRelativeLogFile(c *C) {
	defer saveLog()()
	ctx := getContext(c)
	err := ctx.InitLog(false, false, "logfile")
	c.Assert(err, IsNil)
	log.Printf("hello log")
	raw, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "logfile"))
	c.Assert(err, IsNil)
	c.Assert(string(raw), Matches, ".* JUJU hello log\n")
}
