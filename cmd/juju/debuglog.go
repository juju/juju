// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
)

type DebugLogCommand struct {
	cmd.EnvCommandBase

	lines    int
	entities string
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
	f.StringVar(&c.entities, "e", "", "filter the output by entities (environment, machine or unit)")
	f.StringVar(&c.entities, "entities", "", "")
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

	var entities []string
	if c.entities == "" {
		// Empty entities argument leads to full environment for backward compatability.
		info, err := client.EnvironmentInfo()
		if err != nil {
			return err
		}
		entities = []string{names.EnvironTag(info.UUID)}
	} else {
		// Split argument into entities.
		entities = strings.Split(c.entities, " ")
	}

	debugLog, err := client.WatchDebugLog(c.lines, entities)
	if err != nil {
		return err
	}
	defer debugLog.Close()
	reader := bufio.NewReader(debugLog)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		fmt.Printf(line)
	}
}
