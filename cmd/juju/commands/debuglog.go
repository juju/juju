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

// defaultLineCount is the default number of lines to
// display, from the end of the consolidated log.
const defaultLineCount = 10

var usageDebugLogSummary = `
Displays log messages for a model.`[1:]

var usageDebugLogDetails = `

This command provides access to all logged Juju activity on a per-model
basis. By default, the logs for the currently select model are shown.

Each log line is emitted in this format:

  <entity> <timestamp> <log-level> <module>:<line-no> <message>

The "entity" is the source of the message: a machine or unit. The names for
machines and units can be seen in the output of `[1:] + "`juju status`" + `.

The '--include' and '--exclude' options filter by entity. A unit entity is
identified by prefixing 'unit-' to its corresponding unit name and replacing
the slash with a dash. A machine entity is identified by prefixing 'machine-'
to its corresponding machine id.

The '--include-module' and '--exclude-module' options filter by (dotted)
logging module name. The module name can be truncated such that all loggers
with the prefix will match.

The filtering options combine as follows:
* All --include options are logically ORed together.
* All --exclude options are logically ORed together.
* All --include-module options are logically ORed together.
* All --exclude-module options are logically ORed together.
* The combined --include, --exclude, --include-module and --exclude-module
  selections are logically ANDed to form the complete filter.

Examples:

Exclude all machine 0 messages; show a maximum of 100 lines; and continue to
append filtered messages:

    juju debug-log --exclude machine-0 --lines 100

Include only unit mysql/0 messages; show a maximum of 50 lines; and then
exit:

    juju debug-log -T --include unit-mysql-0 --lines 50

Show all messages from unit apache2/3 or machine 1 and then exit:

    juju debug-log -T --replay --include unit-apache2-3 --include machine-1

Show all juju.worker.uniter logging module messages that are also unit
wordpress/0 messages, and then show any new log messages which match the
filter:

    juju debug-log --replay 
        --include-module juju.worker.uniter \
        --include unit-wordpress-0

Show all messages from the juju.worker.uniter module, except those sent from
machine-3 or machine-4, and then stop:

    juju debug-log --replay --no-tail
        --include-module juju.worker.uniter \
        --exclude machine-3 \
        --exclude machine-4 

To see all WARNING and ERROR messages and then continue showing any
new WARNING and ERROR messages as they are logged:

    juju debug-log --replay --level WARNING

See also: 
    status
    ssh`

func (c *debugLogCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "debug-log",
		Purpose: usageDebugLogSummary,
		Doc:     usageDebugLogDetails,
	}
}

func newDebugLogCommand() cmd.Command {
	return modelcmd.Wrap(&debugLogCommand{})
}

type debugLogCommand struct {
	modelcmd.ModelCommandBase

	level  string
	params api.DebugLogParams
}

func (c *debugLogCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(cmd.NewAppendStringsValue(&c.params.IncludeEntity), "i", "Only show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.IncludeEntity), "include", "Only show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.ExcludeEntity), "x", "Do not show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.ExcludeEntity), "exclude", "Do not show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.IncludeModule), "include-module", "Only show log messages for these logging modules")
	f.Var(cmd.NewAppendStringsValue(&c.params.ExcludeModule), "exclude-module", "Do not show log messages for these logging modules")

	f.StringVar(&c.level, "l", "", "Log level to show, one of [TRACE, DEBUG, INFO, WARNING, ERROR]")
	f.StringVar(&c.level, "level", "", "")

	f.UintVar(&c.params.Backlog, "n", defaultLineCount, "Show this many of the most recent (possibly filtered) lines, and continue to append")
	f.UintVar(&c.params.Backlog, "lines", defaultLineCount, "")
	f.UintVar(&c.params.Limit, "limit", 0, "Exit once this many of the most recent (possibly filtered) lines are shown")
	f.BoolVar(&c.params.Replay, "replay", false, "Show the entire (possibly filtered) log and continue to append")
	f.BoolVar(&c.params.NoTail, "T", false, "Stop after returning existing log messages")
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
