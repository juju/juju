// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"net/rpc"
	"os"
	"path/filepath"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/worker/uniter"
)

var AgentDir = "/var/lib/juju/agents"

type RunCommand struct {
	cmd.CommandBase
	unit     string
	commands string
	showHelp bool
}

const runCommandDoc = `
Run the specified commands in the hook context for the unit.

unit-name can be either the unit tag:
 i.e.  unit-ubuntu-0
or the unit id:
 i.e.  ubuntu/0

The commands are executed with '/bin/bash -s', and the output returned.
`

// Info returns usage information for the command.
func (c *RunCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-run",
		Args:    "<unit-name> <commands>",
		Purpose: "run commands in a unit's hook context",
		Doc:     runCommandDoc,
	}
}

func (c *RunCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.showHelp, "h", false, "show help on juju-run")
	f.BoolVar(&c.showHelp, "help", false, "")
}

func (c *RunCommand) Init(args []string) error {
	// make sure we aren't in an existing hook context
	if contextId, err := getenv("JUJU_CONTEXT_ID"); err == nil && contextId != "" {
		return fmt.Errorf("juju-run cannot be called from within a hook, have context %q", contextId)
	}
	if len(args) < 1 {
		return fmt.Errorf("missing unit-name")
	}
	if len(args) < 2 {
		return fmt.Errorf("missing commands")
	}
	c.unit, args = args[0], args[1:]
	// If the command line param is a unit id (like service/2) we need to
	// change it to the unit tag as that is the format of the agent directory
	// on disk (unit-service-2).
	if names.IsUnit(c.unit) {
		c.unit = names.UnitTag(c.unit)
	}
	c.commands, args = args[0], args[1:]
	return cmd.CheckEmpty(args)
}

func (c *RunCommand) Run(ctx *cmd.Context) error {
	if c.showHelp {
		return gnuflag.ErrHelp
	}

	unitDir := filepath.Join(AgentDir, c.unit)
	logger.Debugf("looking for unit dir %s", unitDir)
	// make sure the unit exists
	_, err := os.Stat(unitDir)
	if os.IsNotExist(err) {
		return fmt.Errorf("unit %q not found on this machine", c.unit)
	} else if err != nil {
		return err
	}

	socketPath := filepath.Join(unitDir, uniter.RunListenerFile)
	// make sure the socket exists
	client, err := rpc.Dial(uniter.RunListenerNetType, socketPath)
	if err != nil {
		return err
	}
	defer client.Close()

	var result cmd.RemoteResponse
	err = client.Call(uniter.JujuRunEndpoint, c.commands, &result)
	if err != nil {
		return err
	}

	ctx.Stdout.Write(result.Stdout)
	ctx.Stderr.Write(result.Stderr)
	return cmd.NewRcPassthroughError(result.Code)
}
