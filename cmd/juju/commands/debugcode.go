// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/network/ssh"
)

func newDebugCodeCommand(hostChecker ssh.ReachableChecker) cmd.Command {
	c := new(debugCodeCommand)
	c.getActionAPI = c.debugHooksCommand.newActionsAPI
	c.setHostChecker(hostChecker)
	return modelcmd.Wrap(c)
}

// debugCodeCommand connects via SSH to a running unit, and drops into a tmux shell,
// prepared to debug hooks/actions as they fire.
type debugCodeCommand struct {
	debugHooksCommand
	debugAt string
}

const debugCodeDoc = `
Interactively debug hooks and actions on a unit.

Similar to 'juju debug-hooks' but rather than dropping you into a shell prompt, 
it runs the hooks and sets the JUJU_DEBUG_AT environment variable. 
Charms that implement support for this should use it to set breakpoints based on the environment
variable.

See the "juju help ssh" for information about SSH related options
accepted by the debug-hooks command.
`

func (c *debugCodeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "debug-code",
		Args:    "<unit name> [hook or action names]",
		Purpose: "Launch a tmux session to debug hooks and/or actions.",
		Doc:     debugCodeDoc,
		Aliases: []string{},
	})
}

func (c *debugCodeCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("no unit name specified")
	}
	c.Target = args[0]
	if !names.IsValidUnit(c.Target) {
		return errors.Errorf("%q is not a valid unit name", c.Target)
	}

	// If any of the hooks is "*", then debug all hooks.
	c.hooks = append([]string{}, args[1:]...)
	for _, h := range c.hooks {
		if h == "*" {
			c.hooks = nil
			break
		}
	}
	return nil
}
func (c *debugCodeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.debugHooksCommand.SetFlags(f)
	f.StringVar(&c.debugAt, "at", "all",
		"interpreted by the charm for where you want to stop, defaults to 'all'")
}

// Run ensures c.Target is a unit, and resolves its address,
// and connects to it via SSH to execute the debug-hooks
// script.
func (c *debugCodeCommand) Run(ctx *cmd.Context) error {
	return c.commonRun(ctx, c.Target, c.hooks, c.debugAt)
}
