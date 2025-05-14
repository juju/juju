// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmdtesting

import (
	"bytes"
	"io/ioutil"

	"github.com/juju/gnuflag"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
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
	f := gnuflag.NewFlagSetWithFlagKnownAs(c.Info().Name, gnuflag.ContinueOnError, cmd.FlagAlias(c, "flag"))
	f.SetOutput(ioutil.Discard)
	c.SetFlags(f)
	if err := f.Parse(c.AllowInterspersedFlags(), args); err != nil {
		return err
	}
	return c.Init(f.Args())
}

// Context creates a simple command execution context with the current
// dir set to a newly created directory within the test directory.
func Context(c tc.LikeC) *cmd.Context {
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdin:  &bytes.Buffer{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	ctx.Context = c.Context()
	return ctx
}

// ContextForDir creates a simple command execution context with the current
// dir set to the specified directory.
func ContextForDir(c tc.LikeC, dir string) *cmd.Context {
	ctx := &cmd.Context{
		Dir:    dir,
		Stdin:  &bytes.Buffer{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	ctx.Context = c.Context()
	return ctx
}

// Stdout takes a command Context that we assume has been created in this
// package, and gets the content of the Stdout buffer as a string.
func Stdout(ctx *cmd.Context) string {
	return ctx.Stdout.(*bytes.Buffer).String()
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
func RunCommand(c tc.LikeC, com cmd.Command, args ...string) (*cmd.Context, error) {
	var context = Context(c)
	return runCommand(context, com, args)
}

// RunCommandInDir works like RunCommand, but runs with a context that uses dir.
func RunCommandInDir(c tc.LikeC, com cmd.Command, args []string, dir string) (*cmd.Context, error) {
	var context = ContextForDir(c, dir)
	return runCommand(context, com, args)
}

func runCommand(ctx *cmd.Context, com cmd.Command, args []string) (*cmd.Context, error) {
	if err := InitCommand(com, args); err != nil {
		cmd.WriteError(ctx.Stderr, err)
		return ctx, err
	}
	return ctx, com.Run(ctx)
}

// RunCommandWithContext runs the command asynchronously with
// the specified context and returns a channel which providers
// the command's errors.
func RunCommandWithContext(ctx *cmd.Context, com cmd.Command, args ...string) chan error {
	if ctx == nil {
		panic("ctx == nil")
	}
	errc := make(chan error, 1)
	go func() {
		if err := InitCommand(com, args); err != nil {
			errc <- err
			return
		}
		errc <- com.Run(ctx)
	}()
	return errc
}

// TestInit checks that a command initialises correctly with the given set of
// arguments.
func TestInit(c tc.LikeC, com cmd.Command, args []string, errPat string) {
	err := InitCommand(com, args)
	if errPat != "" {
		c.Assert(err, tc.ErrorMatches, errPat)
	} else {
		c.Assert(err, tc.IsNil)
	}
}

// HelpText returns a command's formatted help text.
func HelpText(command cmd.Command, name string) string {
	buff := &bytes.Buffer{}
	info := command.Info()
	info.Name = name
	f := gnuflag.NewFlagSetWithFlagKnownAs(info.Name, gnuflag.ContinueOnError, cmd.FlagAlias(command, "flag"))
	command.SetFlags(f)
	buff.Write(info.Help(f))
	return buff.String()
}
