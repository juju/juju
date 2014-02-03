// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"os"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/provider"
)

type DebugLogCommand struct {
	cmd.EnvCommandBase

	lines  int
	filter string
}

// defaultLineCount is the default number of lines to
// display, from the end of the consolidated log.
const defaultLineCount = 10

const debuglogDoc = `
Stream the consolidated debug log file. This file contains the log messages
from all nodes in the environment.
`

func (c *DebugLogCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "debug-log",
		Purpose: "display the consolidated log file",
		Doc:     debuglogDoc,
	}
}

func (c *DebugLogCommand) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&c.lines, "n", defaultLineCount, "output the last K lines; or use -n +K to output lines starting with the Kth")
	f.IntVar(&c.lines, "lines", defaultLineCount, "")
	f.StringVar(&c.filter, "f", "", "filter the output with a regular expression")
	f.StringVar(&c.filter, "filter", "", "")
}

func (c *DebugLogCommand) Init(args []string) error {
	return nil
}

// Run retrieves the debug log via the API.
func (c *DebugLogCommand) Run(ctx *cmd.Context) (err error) {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	logLocation, err := c.logLocation()
	if err != nil {
		return err
	}
	debugLog, err := client.WatchDebugLog(logLocation, c.lines, c.filter)
	if err != nil {
		logger.Infof("WatchDebugLog not supported by the API server, " +
			"falling back to 1.16 compatibility mode using ssh")
		return c.watchDebugLog1dot16(ctx, logLocation)
	}
	defer debugLog.Close()

	_, err = io.Copy(os.Stdout, debugLog)
	return err
}

// watchDebugLog1dot16 runs in case of an older API server and uses ssh
// but with server-side grep.
func (c *DebugLogCommand) watchDebugLog1dot16(ctx *cmd.Context, logLocation string) error {
	// TODO(mue) Testing needed.
	sshCmd := &SSHCommand{}
	tailcmd := fmt.Sprintf("tail -n %d -f %s|grep %s", c.lines, logLocation, c.filter)
	args := append([]string{"0"}, tailcmd)
	err := sshCmd.Init(args)
	if err != nil {
		return err
	}
	return sshCmd.Run(ctx)
}

// logLocation returns the log location for the SSH command based on the provider.
func (c *DebugLogCommand) logLocation() (string, error) {
	store, err := configstore.Default()
	if err != nil {
		return "", fmt.Errorf("cannot open environment info storage: %v", err)
	}
	environ, err := environs.NewFromName(c.EnvironName(), store)
	if err != nil {
		return "", err
	}
	if environ.Config().Type() == provider.Local {
		// Local provider.
		return fmt.Sprintf("%s/%s/log/all-machines.log", osenv.JujuHomeDir(), environ.Name()), nil

	}
	// Default location.
	return "/var/log/juju/all-machines.log", nil
}
