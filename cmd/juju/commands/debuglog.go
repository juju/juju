// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/juju/ansiterm"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/mattn/go-isatty"

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
	return newDebugLogCommandTZ(time.Local)
}

func newDebugLogCommandTZ(tz *time.Location) cmd.Command {
	return modelcmd.Wrap(&debugLogCommand{tz: tz})
}

type debugLogCommand struct {
	modelcmd.ModelCommandBase

	level  string
	params api.DebugLogParams

	utc      bool
	location bool
	date     bool
	ms       bool

	tail   bool
	notail bool
	color  bool

	format string
	tz     *time.Location
}

func (c *debugLogCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
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

	f.BoolVar(&c.notail, "no-tail", false, "Stop after returning existing log messages")
	f.BoolVar(&c.tail, "tail", false, "Wait for new logs")
	f.BoolVar(&c.color, "color", false, "Force use of ANSI color codes")

	f.BoolVar(&c.utc, "utc", false, "Show times in UTC")
	f.BoolVar(&c.location, "location", false, "Show filename and line numbers")
	f.BoolVar(&c.date, "date", false, "Show dates as well as times")
	f.BoolVar(&c.ms, "ms", false, "Show times to millisecond precision")
}

func (c *debugLogCommand) Init(args []string) error {
	if c.level != "" {
		level, ok := loggo.ParseLevel(c.level)
		if !ok || level < loggo.TRACE || level > loggo.ERROR {
			return errors.Errorf("level value %q is not one of %q, %q, %q, %q, %q",
				c.level, loggo.TRACE, loggo.DEBUG, loggo.INFO, loggo.WARNING, loggo.ERROR)
		}
		c.params.Level = level
	}
	if c.tail && c.notail {
		return errors.NotValidf("setting --tail and --no-tail")
	}
	if c.utc {
		c.tz = time.UTC
	}
	if c.date {
		c.format = "2006-01-02 15:04:05"
	} else {
		c.format = "15:04:05"
	}
	if c.ms {
		c.format = c.format + ".000"
	}
	return cmd.CheckEmpty(args)
}

type DebugLogAPI interface {
	WatchDebugLog(params api.DebugLogParams) (<-chan api.LogMessage, error)
	Close() error
}

var getDebugLogAPI = func(c *debugLogCommand) (DebugLogAPI, error) {
	return c.NewAPIClient()
}

func isTerminal(out io.Writer) bool {
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd())
}

// Run retrieves the debug log via the API.
func (c *debugLogCommand) Run(ctx *cmd.Context) (err error) {
	if c.tail {
		c.params.NoTail = false
	} else if c.notail {
		c.params.NoTail = true
	} else {
		// Set the default tail option to true if the caller is
		// using a terminal.
		c.params.NoTail = !isTerminal(ctx.Stdout)
	}

	client, err := getDebugLogAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()
	messages, err := client.WatchDebugLog(c.params)
	if err != nil {
		return err
	}
	writer := ansiterm.NewWriter(ctx.Stdout)
	if c.color {
		writer.SetColorCapable(true)
	}
	for {
		msg, ok := <-messages
		if !ok {
			break
		}
		c.writeLogRecord(writer, msg)
	}

	return nil
}

var SeverityColor = map[string]*ansiterm.Context{
	"TRACE":   ansiterm.Foreground(ansiterm.Default),
	"DEBUG":   ansiterm.Foreground(ansiterm.Green),
	"INFO":    ansiterm.Foreground(ansiterm.BrightBlue),
	"WARNING": ansiterm.Foreground(ansiterm.Yellow),
	"ERROR":   ansiterm.Foreground(ansiterm.BrightRed),
	"CRITICAL": &ansiterm.Context{
		Foreground: ansiterm.White,
		Background: ansiterm.Red,
	},
}

func (c *debugLogCommand) writeLogRecord(w *ansiterm.Writer, r api.LogMessage) {
	ts := r.Timestamp.In(c.tz).Format(c.format)
	fmt.Fprintf(w, "%s: %s ", r.Entity, ts)
	SeverityColor[r.Severity].Fprintf(w, r.Severity)
	fmt.Fprintf(w, " %s ", r.Module)
	if c.location {
		loggo.LocationColor.Fprintf(w, "%s ", r.Location)
	}
	fmt.Fprintln(w, r.Message)
}
