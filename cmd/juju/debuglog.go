// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"io"
	"os"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
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
Stream the consolidated log file. The consolidated log file contains log messages
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
		return err
	}
	defer debugLog.Close()

	_, err = io.Copy(os.Stdout, debugLog)
	return err
}
