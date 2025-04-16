// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

func newHelpActionCmdsCommand() cmd.Command {
	return &helpActionCmdsCommand{}
}

type helpActionCmdsCommand struct {
	cmd.CommandBase
	actionCmd string
}

func (t *helpActionCmdsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "help-action-commands",
		Args:     "[action]",
		Purpose:  "Show help on a Juju charm action command.",
		Doc:      helpActionCmdsDoc,
		Examples: helpActionCmdsExamples,
		SeeAlso:  []string{"help", "help-hook-commands"},
	})
}

func (t *helpActionCmdsCommand) Init(args []string) error {
	actionCmd, err := cmd.ZeroOrOneArgs(args)
	if err == nil {
		t.actionCmd = actionCmd
	}
	return err
}

func (c *helpActionCmdsCommand) Run(ctx *cmd.Context) error {
	if c.actionCmd == "" {
		fmt.Fprint(ctx.Stdout, listHelpActionCmds())
	} else {
		c, err := jujuc.NewActionCommandForHelp(dummyHookContext{}, c.actionCmd)
		if err != nil {
			return err
		}
		info := c.Info()
		f := gnuflag.NewFlagSetWithFlagKnownAs(info.Name, gnuflag.ContinueOnError, cmd.FlagAlias(c, "option"))
		c.SetFlags(f)
		_, _ = ctx.Stdout.Write(info.Help(f))
	}
	return nil
}

var helpActionCmdsDoc = fmt.Sprintf(`
In addition to hook commands, Juju charms also have access to a set of action-specific commands.
These action commands are available when an action is running, and are used to log progress
and report the outcome of the action.
The currently available charm action commands include:
%v
`, listHelpActionCmds())

const helpActionCmdsExamples = `
For help on a specific action command, supply the name of that action command, for example:

        juju help-action-commands action-fail
`

func listHelpActionCmds() string {
	all := ""
	// Ripped from SuperCommand. We could Run() a SuperCommand
	// with "help commands", but then the implicit "help" command
	// shows up.
	names := jujuc.ActionCommandNames()
	cmds := []cmd.Command{}
	longest := 0
	for _, name := range names {
		if c, err := jujuc.NewActionCommandForHelp(dummyHookContext{}, name); err == nil {
			// On Windows name has a '.exe' suffix, while Info().Name does not
			name := c.Info().Name
			if len(name) > longest {
				// Left-aligns the command purpose to match the longest command name length.
				longest = len(name)
			}
			cmds = append(cmds, c)
		}
	}
	for _, c := range cmds {
		info := c.Info()
		all += fmt.Sprintf("    %-*s  %s\n", longest, info.Name, info.Purpose)
	}
	return all
}
