// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"io/ioutil"
	"strings"

	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
	jujucmd "github.com/juju/juju/cmd/juju"
	actioncmd "github.com/juju/juju/cmd/juju/action"
	servicecmd "github.com/juju/juju/cmd/juju/service"
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
	return ctx.Stdout.(*bytes.Buffer).String()
}

// Stderr takes a command Context that we assume has been created in this
// package, and gets the content of the Stderr buffer as a string.
func Stderr(ctx *cmd.Context) string {
	return ctx.Stderr.(*bytes.Buffer).String()
}

// FindCommand returns the appropriate juju command for the provided
// command name. If a subcommand name is provided then it is looked up
// relative to the named command. Environment-based commands are
// appropriately wrapped.
func FindCommand(c *gc.C, name, sub string) cmd.Command {
	parts := strings.Fields(name)
	c.Assert(parts, gc.Not(gc.HasLen), 0)

	// TODO(ericsnow) Instead, use the registry in cmd/juju/main.go...
	var command cmd.Command
	switch name {
	case "action":
		switch sub {
		case "do":
			command = &actioncmd.DoCommand{}
		case "fetch":
			command = &actioncmd.FetchCommand{}
		}
	case "deploy":
		c.Assert(sub, gc.Equals, "")
		command = &jujucmd.DeployCommand{}
	case "destroy-environment":
		c.Assert(sub, gc.Equals, "")
		command = &jujucmd.DestroyEnvironmentCommand{}
	case "destroy-unit":
		c.Assert(sub, gc.Equals, "")
		command = &jujucmd.RemoveUnitCommand{}
	case "service":
		switch sub {
		case "add":
			command = &servicecmd.AddUnitCommand{}
		case "set":
			command = &servicecmd.SetCommand{}
		}
	}
	if command == nil {
		c.Errorf("command not recognized: juju %s %s", name, sub)
		c.Fail()
	}
	if environCmd, ok := command.(envcmd.EnvironCommand); ok {
		command = envcmd.Wrap(environCmd)
	}
	return command
}

// RunCommand runs a command with the specified args.  The returned error
// may come from either the parsing of the args, the command initialisation, or
// the actual running of the command.  Access to the resulting output streams
// is provided through the returned context instance.
func RunCommand(c *gc.C, com cmd.Command, args ...string) (*cmd.Context, error) {
	if err := InitCommand(com, args); err != nil {
		return nil, err
	}
	var context = Context(c)
	return context, com.Run(context)
}

// RunCommandString runs a command with the specified args. The only
// difference from RunCommand is that the command to run is derived from
// the provided command name. The command name may be a single name or
// a super command and a sub-command.
func RunCommandString(c *gc.C, commandName string, args ...string) (*cmd.Context, error) {
	parts := strings.SplitN(commandName, " ", 2)
	commandName = parts[0]
	subName := ""
	if len(parts) == 2 {
		subName = parts[1]
	}
	command := FindCommand(c, commandName, subName)

	return RunCommand(c, command, args...)
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
