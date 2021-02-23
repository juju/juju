// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/network/ssh"
)

func newDebugCodeCommand(hostChecker ssh.ReachableChecker) cmd.Command {
	c := new(debugCodeCommand)
	c.hostChecker = hostChecker
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

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

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
	return c.debugHooksCommand.Init(args)
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
	if err := c.initAPIs(); err != nil {
		return err
	}
	defer c.closeAPIs()
	return c.commonRun(ctx, c.provider.getTarget(), c.hooks, c.debugAt)
}
