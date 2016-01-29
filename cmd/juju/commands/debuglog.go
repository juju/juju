// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
)

func newDebugLogCommand() cmd.Command {
	return modelcmd.Wrap(&debugLogCommand{})
}

type debugLogCommand struct {
	modelcmd.ModelCommandBase

	level  string
	params api.DebugLogParams
}

// defaultLineCount is the default number of lines to
// display, from the end of the consolidated log.
const defaultLineCount = 10

const debuglogDoc = `
Stream the consolidated debug log file. This file contains the log messages
from all nodes in the model.
`

func (c *debugLogCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "debug-log",
		Purpose: "display the consolidated log file",
		Doc:     debuglogDoc,
	}
}

func (c *debugLogCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(cmd.NewAppendStringsValue(&c.params.IncludeEntity), "i", "only show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.IncludeEntity), "include", "only show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.ExcludeEntity), "x", "do not show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.ExcludeEntity), "exclude", "do not show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.IncludeModule), "include-module", "only show log messages for these logging modules")
	f.Var(cmd.NewAppendStringsValue(&c.params.ExcludeModule), "exclude-module", "do not show log messages for these logging modules")

	f.StringVar(&c.level, "l", "", "log level to show, one of [TRACE, DEBUG, INFO, WARNING, ERROR]")
	f.StringVar(&c.level, "level", "", "")

	f.UintVar(&c.params.Backlog, "n", defaultLineCount, "go back this many lines from the end before starting to filter")
	f.UintVar(&c.params.Backlog, "lines", defaultLineCount, "")
	f.UintVar(&c.params.Limit, "limit", 0, "show at most this many lines")
	f.BoolVar(&c.params.Replay, "replay", false, "start filtering from the start")
	f.BoolVar(&c.params.NoTail, "T", false, "stop after returning existing log messages")
	f.BoolVar(&c.params.NoTail, "no-tail", false, "")
}

func (c *debugLogCommand) Init(args []string) error {
	if c.level != "" {
		level, ok := loggo.ParseLevel(c.level)
		if !ok || level < loggo.TRACE || level > loggo.ERROR {
			return fmt.Errorf("level value %q is not one of %q, %q, %q, %q, %q",
				c.level, loggo.TRACE, loggo.DEBUG, loggo.INFO, loggo.WARNING, loggo.ERROR)
		}
		c.params.Level = level
	}
	return cmd.CheckEmpty(args)
}

type DebugLogAPI interface {
	WatchDebugLog(params api.DebugLogParams) (io.ReadCloser, error)
	Close() error
}

var getDebugLogAPI = func(c *debugLogCommand) (DebugLogAPI, error) {
	return c.NewAPIClient()
}

// Run retrieves the debug log via the API.
func (c *debugLogCommand) Run(ctx *cmd.Context) (err error) {
	client, err := getDebugLogAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()
	debugLog, err := client.WatchDebugLog(c.params)
	if err != nil {
		return err
	}
	defer debugLog.Close()
	_, err = io.Copy(ctx.Stdout, debugLog)
	return err
}

var runSSHCommand = func(sshCmd *sshCommand, ctx *cmd.Context) error {
	return sshCmd.Run(ctx)
}
