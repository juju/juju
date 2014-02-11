// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

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

	debugLog, err := client.WatchDebugLog(c.lines, c.filter)
	if err != nil {
		logger.Infof("WatchDebugLog not supported by the API server, " +
			"falling back to 1.16 compatibility mode using ssh")
		return c.watchDebugLog1dot16(ctx)
	}
	defer debugLog.Close()

	_, err = io.Copy(os.Stdout, debugLog)
	return err
}

// watchDebugLog1dot16 runs in case of an older API server and uses ssh
// but with server-side grep.
func (c *DebugLogCommand) watchDebugLog1dot16(ctx *cmd.Context) error {
	name, local, err := c.currentEnvironment()
	if err != nil {
		return err
	}
	// Work depending on the provider.
	if local {
		// Local provider tails local log file.
		logLocation := fmt.Sprintf("%s/%s/log/all-machines.log", osenv.JujuHomeDir(), name)
		tailCmd := exec.Command("tail", "-n", strconv.Itoa(c.lines), "-f", logLocation)
		grepCmd := exec.Command("grep", "-E", c.filter)
		r, w := io.Pipe()

		tailCmd.Stdout = w
		grepCmd.Stdin = r
		grepCmd.Stdout = os.Stdout
		grepCmd.Stderr = os.Stderr

		err := tailCmd.Start()
		if err != nil {
			return err
		}
		err = grepCmd.Start()
		if err != nil {
			return err
		}
		return grepCmd.Wait()
	}
	// Any other provider uses ssh with tail.
	logLocation := "/var/log/juju/all-machines.log"
	sshCmd := &SSHCommand{}
	tailGrepCmd := fmt.Sprintf("tail -n %d -f %s|grep %s", c.lines, logLocation, c.filter)
	args := []string{"0", tailGrepCmd}
	err = sshCmd.Init(args)
	if err != nil {
		return err
	}
	return sshCmd.Run(ctx)
}

// currentEnvironment returns the name of the environment and if it is local.
func (c *DebugLogCommand) currentEnvironment() (string, bool, error) {
	store, err := configstore.Default()
	if err != nil {
		return "", false, fmt.Errorf("cannot open environment info storage: %v", err)
	}
	environ, err := environs.NewFromName(c.EnvironName(), store)
	if err != nil {
		return "", false, err
	}
	local := environ.Config().Type() == provider.Local
	return environ.Name(), local, nil
}
