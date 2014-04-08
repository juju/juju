// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"os"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

type DebugLogCommand struct {
	cmd.EnvCommandBase

	include       []string
	exclude       []string
	includeModule []string
	excludeModule []string
	limit         uint
	lines         uint
	level         string
	replay        bool
}

var DefaultLogLocation = "/var/log/juju/all-machines.log"

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
	f.Var(cmd.NewAppendStringsValue(&c.include), "i", "only show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.include), "include", "only show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.exclude), "x", "only show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.exclude), "exclude", "only show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.includeModule), "include-module", "only show log messages for these logging modules")
	f.Var(cmd.NewAppendStringsValue(&c.excludeModule), "exclude-module", "do not show log messages for these logging modules")

	f.StringVar(&c.level, "l", "", "log level to show, one of [TRACE, DEBUG, INFO, WARNING, ERROR]")
	f.StringVar(&c.level, "level", "", "")

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

	logger.Debugf("CALLING WATCH DEBUG LOG")
	debugLog, err := client.WatchDebugLog(c.lines, c.filter)
	if err != nil {
		logger.Infof("WatchDebugLog not supported by the API server, "+
			"falling back to 1.16 compatibility mode using ssh: %v", err)
		return c.watchDebugLog1dot16(ctx)
	}
	defer debugLog.Close()

	_, err = io.Copy(os.Stdout, debugLog)
	return err
}

// watchDebugLog1dot16 runs in case of an older API server and uses ssh
// but with server-side grep.
func (c *DebugLogCommand) watchDebugLog1dot16(ctx *cmd.Context) error {
	sshCmd := &SSHCommand{}
	tailGrepCmd := fmt.Sprintf("tail -n %d -f %s", c.lines, DefaultLogLocation)
	if c.filter != "" {
		tailGrepCmd += fmt.Sprintf("  | grep -E %s", c.filter)
	}
	args := []string{"0", tailGrepCmd}
	err := sshCmd.Init(args)
	if err != nil {
		return err
	}
	return sshCmd.Run(ctx)
}
