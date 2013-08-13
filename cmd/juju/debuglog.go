// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

type DebugLogCommand struct {
	// The debug log command simply invokes juju ssh with the required arguments.
	sshCmd cmd.Command
}

const debuglogDoc = `
Launch an ssh shell on the state server machine and tail the consolidated log file.
The consolidated log file contains log messages from all nodes in the environment.
`

func (c *DebugLogCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "debug-log",
		Args:    "[<ssh args>...]",
		Purpose: "display the consolidated log file",
		Doc:     debuglogDoc,
	}
}

func (c *DebugLogCommand) SetFlags(f *gnuflag.FlagSet) {
	c.sshCmd.SetFlags(f)
}

func (c *DebugLogCommand) AllowInterspersedFlags() bool {
	return c.sshCmd.AllowInterspersedFlags()
}

func (c *DebugLogCommand) Init(args []string) error {
	args = append([]string{"0"}, args...)
	args = append(args, "tail -f /var/log/juju/all-machines.log")
	return c.sshCmd.Init(args)
}

// Run uses "juju ssh" to log into the state server node
// and tails the consolidated log file which captures log
// messages from all nodes.
func (c *DebugLogCommand) Run(ctx *cmd.Context) error {
	return c.sshCmd.Run(ctx)
}
