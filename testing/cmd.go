// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/gnuflag"
)

// NewFlagSet creates a new flag set using the standard options, particularly
// the option to stop the gnuflag methods from writing to StdErr or StdOut.
func NewFlagSet() *gnuflag.FlagSet {
	fs := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	fs.SetOutput(ioutil.Discard)
	return fs
}

// InitCommand will create a new flag set, and call the Command's SetFlags and
// Init methods with the appropriate args.
func InitCommand(c cmd.Command, args []string) error {
	f := NewFlagSet()
	c.SetFlags(f)
	if err := f.Parse(c.AllowInterspersedFlags(), args); err != nil {
		return err
	}
	return c.Init(f.Args())
}

// Context creates a simple command execution context with the current
// dir set to a newly created directory within the test directory.
func Context(c *gc.C) *cmd.Context {
	return &cmd.Context{
		Dir:    c.MkDir(),
		Stdin:  &bytes.Buffer{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
}

// ContextForDir creates a simple command execution context with the current
// dir set to the specified directory.
func ContextForDir(c *gc.C, dir string) *cmd.Context {
	return &cmd.Context{
		Dir:    dir,
		Stdin:  &bytes.Buffer{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
}

// Stdout takes a command Context that we assume has been created in this
// package, and gets the content of the Stdout buffer as a string.
func Stdout(ctx *cmd.Context) string {
	buff, ok := ctx.Stdout.(*bytes.Buffer)
	if ok {
		return buff.String()
	}
	return "not a buffer"
}

// Stderr takes a command Context that we assume has been created in this
// package, and gets the content of the Stderr buffer as a string.
func Stderr(ctx *cmd.Context) string {
	return ctx.Stderr.(*bytes.Buffer).String()
}

// RunCommand runs a command with the specified args.  The returned error
// may come from either the parsing of the args, the command initialisation, or
// the actual running of the command.  Access to the resulting output streams
// is provided through the returned context instance.
func RunCommand(c *gc.C, com cmd.Command, args ...string) (*cmd.Context, error) {
	var context = Context(c)
	if err := InitCommand(com, args); err != nil {
		if err != nil {
			fmt.Fprintf(context.Stderr, "error: %v\n", err)
		}
		return context, err
	}
	return context, com.Run(context)
}

// RunCommandInDir works like RunCommand, but runs with a context that uses dir.
func RunCommandInDir(c *gc.C, com cmd.Command, args []string, dir string) (*cmd.Context, error) {
	if err := InitCommand(com, args); err != nil {
		return nil, err
	}
	var context = ContextForDir(c, dir)
	return context, com.Run(context)
}

// TestInit checks that a command initialises correctly with the given set of
// arguments.
func TestInit(c *gc.C, com cmd.Command, args []string, errPat string) {
	err := InitCommand(com, args)
	if errPat != "" {
		c.Assert(err, gc.ErrorMatches, errPat)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

// ExtractCommandsFromHelpOutput takes the standard output from the
// command context and looks for the "commands:" string and returns the
// commands output after that.
func ExtractCommandsFromHelpOutput(ctx *cmd.Context) []string {
	var namesFound []string
	commandHelp := strings.SplitAfter(Stdout(ctx), "commands:")[1]
	commandHelp = strings.TrimSpace(commandHelp)
	for _, line := range strings.Split(commandHelp, "\n") {
		namesFound = append(namesFound, strings.TrimSpace(strings.Split(line, " - ")[0]))
	}
	return namesFound
}
