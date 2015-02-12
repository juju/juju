// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/exec"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter"
)

type RunCommand struct {
	cmd.CommandBase
	unit            names.UnitTag
	commands        string
	showHelp        bool
	noContext       bool
	forceRemoteUnit bool
	relationId      string
	remoteUnitName  string
}

const runCommandDoc = `
Run the specified commands in the hook context for the unit.

unit-name can be either the unit tag:
 i.e.  unit-ubuntu-0
or the unit id:
 i.e.  ubuntu/0

If --no-context is specified, the <unit-name> positional
argument is not needed.

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
	f.BoolVar(&c.noContext, "no-context", false, "do not run the command in a unit context")
	f.StringVar(&c.relationId, "r", "", "run the commands for a specific relation context on a unit")
	f.StringVar(&c.relationId, "relation", "", "")
	f.StringVar(&c.remoteUnitName, "remote-unit", "", "run the commands for a specific remote unit in a relation context on a unit")
	f.BoolVar(&c.forceRemoteUnit, "force-remote-unit", false, "run the commands for a specific relation context, bypassing the remote unit check")
}

func (c *RunCommand) Init(args []string) error {
	// make sure we aren't in an existing hook context
	if contextId, err := getenv("JUJU_CONTEXT_ID"); err == nil && contextId != "" {
		return fmt.Errorf("juju-run cannot be called from within a hook, have context %q", contextId)
	}
	if !c.noContext {
		if len(args) < 1 {
			return fmt.Errorf("missing unit-name")
		}
		var unitName string
		unitName, args = args[0], args[1:]
		// If the command line param is a unit id (like service/2) we need to
		// change it to the unit tag as that is the format of the agent directory
		// on disk (unit-service-2).
		if names.IsValidUnit(unitName) {
			c.unit = names.NewUnitTag(unitName)
		} else {
			var err error
			c.unit, err = names.ParseUnitTag(unitName)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	if len(args) < 1 {
		return fmt.Errorf("missing commands")
	}
	c.commands, args = args[0], args[1:]
	return cmd.CheckEmpty(args)
}

func (c *RunCommand) Run(ctx *cmd.Context) error {
	var result *exec.ExecResponse
	var err error
	if c.noContext {
		result, err = c.executeNoContext()
	} else {
		result, err = c.executeInUnitContext()
	}
	if err != nil {
		return errors.Trace(err)
	}

	ctx.Stdout.Write(result.Stdout)
	ctx.Stderr.Write(result.Stderr)
	return cmd.NewRcPassthroughError(result.Code)
}

func (c *RunCommand) socketPath() string {
	paths := uniter.NewPaths(cmdutil.DataDir, c.unit)
	return paths.Runtime.JujuRunSocket
}

func (c *RunCommand) executeInUnitContext() (*exec.ExecResponse, error) {
	unitDir := agent.Dir(cmdutil.DataDir, c.unit)
	logger.Debugf("looking for unit dir %s", unitDir)
	// make sure the unit exists
	_, err := os.Stat(unitDir)
	if os.IsNotExist(err) {
		return nil, errors.Errorf("unit %q not found on this machine", c.unit.Id())
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	relationId, err := checkRelationId(c.relationId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(c.remoteUnitName) > 0 && relationId == -1 {
		return nil, errors.Errorf("remote unit: %s, provided without a relation", c.remoteUnitName)
	}
	client, err := sockets.Dial(c.socketPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()

	var result exec.ExecResponse
	args := uniter.RunCommandsArgs{
		Commands:        c.commands,
		RelationId:      relationId,
		RemoteUnitName:  c.remoteUnitName,
		ForceRemoteUnit: c.forceRemoteUnit,
	}
	err = client.Call(uniter.JujuRunEndpoint, args, &result)
	return &result, errors.Trace(err)
}

// appendProxyToCommands activates proxy settings on platforms
// that support this feature via the command line. Currently this
// will work on most GNU/Linux systems, but has no use in Windows
// where the proxy settings are taken from the registry or from
// application specific settings (proxy settings in firefox ignore
// registry values on Windows).
func (c *RunCommand) appendProxyToCommands() string {
	switch version.Current.OS {
	case version.Ubuntu:
		return `[ -f "/home/ubuntu/.juju-proxy" ] && . "/home/ubuntu/.juju-proxy"` + "\n" + c.commands
	default:
		return c.commands
	}
}

func (c *RunCommand) executeNoContext() (*exec.ExecResponse, error) {
	// Acquire the uniter hook execution lock to make sure we don't
	// stomp on each other.
	lock, err := cmdutil.HookExecutionLock(cmdutil.DataDir)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = lock.Lock("juju-run")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer lock.Unlock()

	runCmd := c.appendProxyToCommands()

	return exec.RunCommands(
		exec.RunParams{
			Commands: runCmd,
		})
}

// checkRelationId verifies that the relationId
// given by the user is of a valid syntax, it does
// not check that the relationId is a valid one. This
// is done by the NewRunner method that is part of
// the worker/uniter/runner/factory package.
func checkRelationId(value string) (int, error) {
	if len(value) == 0 {
		return -1, nil
	}

	trim := value
	if idx := strings.LastIndex(trim, ":"); idx != -1 {
		trim = trim[idx+1:]
	}
	id, err := strconv.Atoi(trim)
	if err != nil {
		return -1, errors.Errorf("invalid relation id")
	}
	return id, nil
}
