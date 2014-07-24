// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/names"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/fslock"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter"
)

var (
	AgentDir = filepath.Join(dataDir, "agents")
	LockDir  = filepath.Join(dataDir, "locks")
)

type RunCommand struct {
	cmd.CommandBase
	unit      string
	commands  string
	showHelp  bool
	noContext bool
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
	f.BoolVar(&c.showHelp, "h", false, "show help on juju-run")
	f.BoolVar(&c.showHelp, "help", false, "")
	f.BoolVar(&c.noContext, "no-context", false, "do not run the command in a unit context")
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
		c.unit, args = args[0], args[1:]
		// If the command line param is a unit id (like service/2) we need to
		// change it to the unit tag as that is the format of the agent directory
		// on disk (unit-service-2).
		if names.IsValidUnit(c.unit) {
			c.unit = names.NewUnitTag(c.unit).String()
		}
	}
	if len(args) < 1 {
		return fmt.Errorf("missing commands")
	}
	c.commands, args = args[0], args[1:]
	return cmd.CheckEmpty(args)
}

func (c *RunCommand) Run(ctx *cmd.Context) error {
	if c.showHelp {
		return gnuflag.ErrHelp
	}

	var result *exec.ExecResponse
	var err error
	if c.noContext {
		result, err = c.executeNoContext()
	} else {
		result, err = c.executeInUnitContext()
	}
	if err != nil {
		return err
	}

	ctx.Stdout.Write(result.Stdout)
	ctx.Stderr.Write(result.Stderr)
	return cmd.NewRcPassthroughError(result.Code)
}

func (c *RunCommand) nixSockPath() string {
	unitDir := filepath.Join(AgentDir, c.unit)
	return filepath.Join(unitDir, uniter.RunListenerFile)
}

func (c *RunCommand) winSockPath() string {
	return fmt.Sprintf(`\\.\pipe\%s-run`, c.unit)
}

func (c *RunCommand) sockPath() string {
	switch version.Current.OS {
	case version.Windows:
		return c.winSockPath()
	default:
		return c.nixSockPath()
	}
}

func (c *RunCommand) executeInUnitContext() (*exec.ExecResponse, error) {
	unitDir := filepath.Join(AgentDir, c.unit)
	logger.Debugf("looking for unit dir %s", unitDir)
	// make sure the unit exists
	_, err := os.Stat(unitDir)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("unit %q not found on this machine", c.unit)
	} else if err != nil {
		return nil, err
	}
	// make sure the socket exists
	socketPath := c.sockPath()
	client, err := sockets.Dial(socketPath)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	var result exec.ExecResponse
	err = client.Call(uniter.JujuRunEndpoint, c.commands, &result)
	return &result, err
}

func getLock() (*fslock.Lock, error) {
	return fslock.NewLock(LockDir, "uniter-hook-execution")
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
	lock, err := getLock()
	if err != nil {
		return nil, err
	}
	err = lock.Lock("juju-run")
	if err != nil {
		return nil, err
	}
	defer lock.Unlock()

	runCmd := c.appendProxyToCommands()

	return exec.RunCommands(
		exec.RunParams{
			Commands: runCmd,
		})
}
