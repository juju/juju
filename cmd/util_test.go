// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
)

func bufferString(stream io.Writer) string {
	return stream.(*bytes.Buffer).String()
}

// TestCommand is used by several different tests.
type TestCommand struct {
	cmd.CommandBase
	Name    string
	Option  string
	Minimal bool
	Aliases []string
}

func (c *TestCommand) Info() *cmd.Info {
	if c.Minimal {
		return &cmd.Info{Name: c.Name}
	}
	return &cmd.Info{
		Name:    c.Name,
		Args:    "<something>",
		Purpose: c.Name + " the juju",
		Doc:     c.Name + "-doc",
		Aliases: c.Aliases,
	}
}

func (c *TestCommand) SetFlags(f *gnuflag.FlagSet) {
	if !c.Minimal {
		f.StringVar(&c.Option, "option", "", "option-doc")
	}
}

func (c *TestCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
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
