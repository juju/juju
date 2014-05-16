// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api"
)

type DebugLogCommand struct {
	envcmd.EnvCommandBase

	level  string
	params api.DebugLogParams
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
}

func (c *DebugLogCommand) Init(args []string) error {
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

var getDebugLogAPI = func(envName string) (DebugLogAPI, error) {
	return juju.NewAPIClientFromName(envName)
}

// Run retrieves the debug log via the API.
func (c *DebugLogCommand) Run(ctx *cmd.Context) (err error) {
	client, err := getDebugLogAPI(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	debugLog, err := client.WatchDebugLog(c.params)
	if err != nil {
		if errors.IsNotSupported(err) {
			return c.watchDebugLog1dot18(ctx)
		}
		return err
	}
	defer debugLog.Close()
	_, err = io.Copy(ctx.Stdout, debugLog)
	return err
}

var runSSHCommand = func(sshCmd *SSHCommand, ctx *cmd.Context) error {
	return sshCmd.Run(ctx)
}

// watchDebugLog1dot18 runs in case of an older API server and uses ssh
// but with server-side grep.
func (c *DebugLogCommand) watchDebugLog1dot18(ctx *cmd.Context) error {
	ctx.Infof("Server does not support new stream log, falling back to tail")
	ctx.Verbosef("filters are not supported with tail")
	sshCmd := &SSHCommand{}
	tailCmd := fmt.Sprintf("tail -n -%d -f %s", c.params.Backlog, DefaultLogLocation)
	// If the api doesn't support WatchDebugLog, then it won't be running in
	// HA either, so machine 0 is where it is all at.
	args := []string{"0", tailCmd}
	err := sshCmd.Init(args)
	if err != nil {
		return err
	}
	sshCmd.EnvName = c.EnvName
	return runSSHCommand(sshCmd, ctx)
}
