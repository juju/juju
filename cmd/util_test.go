package cmd_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
)

func dummyFlagSet() *gnuflag.FlagSet {
	return gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
}

func dummyContext(c *C) *cmd.Context {
	return &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}}
}

func bufferString(stream io.Writer) string {
	return stream.(*bytes.Buffer).String()
}

// TestCommand is used by several different tests.
type TestCommand struct {
	Name    string
	Option  string
	Minimal bool
}

func (c *TestCommand) Info() *cmd.Info {
	if c.Minimal {
		return &cmd.Info{c.Name, "", "", ""}
	}
	return &cmd.Info{c.Name, "<something>", c.Name + " the juju", c.Name + "-doc"}
}

func (c *TestCommand) Init(f *gnuflag.FlagSet, args []string) error {
	if !c.Minimal {
		f.StringVar(&c.Option, "option", "", "option-doc")
	}
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *TestCommand) Run(ctx *cmd.Context) error {
	switch c.Option {
	case "error":
		return errors.New("BAM!")
	case "silent-error":
		return cmd.ErrSilent
	case "echo":
		_, err := io.Copy(ctx.Stdout, ctx.Stdin)
		return err
	default:
		fmt.Fprintln(ctx.Stdout, c.Option)
	}
	return nil
}

// minimalHelp and fullHelp are the expected help strings for a TestCommand
// with Name "verb", with and without Minimal set.
var minimalHelp = "usage: verb\n"
var optionHelp = `usage: verb [options] <something>
purpose: verb the juju

options:
--option (= "")
    option-doc
`
var fullHelp = `usage: verb [options] <something>
purpose: verb the juju

options:
--option (= "")
    option-doc

verb-doc
`
