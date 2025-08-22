// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/ansiterm"
	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/loggo/loggocolor"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/mattn/go-isatty"

	"github.com/juju/juju/api/common"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
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

The ` + "`<entity>`" + ` is the source of the message: a machine or a unit. The names for
machines and units can be seen in the output of `[1:] + "`juju status`" + `.

The ` + "`--include`" + ` and ` + "`--exclude`" + ` options filter by entity. The entity can be
a machine, unit, or application for VM models, but can be application only
for Kubernetes models. These filters support wildcards ` + "`*`" + ` if filtering on the
entity full name (prefixed by ` + "`<entity type>-`" + `).

The ` + "`--include-module`" + ` and ` + "`--exclude-module`" + ` options filter by the (dotted)
logging module name. The module name can be truncated such that all loggers
with the prefix will match.

The ` + "`--include-label`" + ` and ` + "`--exclude-label`" + ` options filter by logging label.

The filtering options combine as follows:
* All ` + "`--include`" + ` options are logically ORed together.
* All ` + "`--exclude`" + ` options are logically ORed together.
* All ` + "`--include-module`" + ` options are logically ORed together.
* All ` + "`--exclude-module`" + ` options are logically ORed together.
* All ` + "`--include-label`" + ` options are logically ORed together.
* All ` + "`--exclude-label`" + ` options are logically ORed together.
* The combined ` + "`--include`" + `, ` + "`--exclude`" + `, ` + "`--include-module`" + `, ` + "`--exclude-module`" + `,
  ` + "`--include-label`" + ` and ` + "`--exclude-label`" + ` selections are logically ANDed to form
  the complete filter.

The ` + "`--tail`" + ` option waits for and continuously prints new log lines after displaying the most recent log lines.

The ` + "`--no-tail`" + ` option displays the most recent log lines and then exits immediately.

The ` + "`--lines`" + ` and ` + "`--limit`" + ` options control the number of log lines displayed:
* the ` + "`--lines`" + ` option prints the specified number of the most recent lines and then waits for new lines. This implies --tail.
* the ` + "`--limit`" + ` option prints up to the specified number of the most recent lines and exits. This implies --no-tail.
* setting ` + "`--lines`" + ` or ` + "`--limit`" + ` to 0 will print the maximum number of the most recent lines available.

The ` + "`--replay`" + ` option displays log lines starting from the beginning.

Behavior when combining ` + "`--replay`" + ` with other options:
* ` + "`--replay`" + ` and ` + "`--limit`" + ` prints the specified number of lines from the beginning of the log.
* ` + "`--replay`" + ` and ` + "`--lines`" + ` is invalid as it causes confusion by skipping logs between the replayed lines and the current tailing point.

Given the above, the following flag combinations are incompatible and cannot be specified together:
* ` + "`--tail`" + ` and ` + "`--no-tail`" + `
* ` + "`--tail`" + ` and ` + "`--limit`" + `
* ` + "`--no-tail`" + ` and ` + "`--lines (-n)`" + `
* ` + "`--limit`" + ` and ` + "`--lines (-n)`" + `
* ` + "`--replay`" + ` and ` + "`--lines (-n)`" + `
`

const usageDebugLogExamples = `

Begin with all the log messages:

    juju debug-log --replay

Begin with the last 500 lines, using grep as a text filter:

    juju debug-log -n 500 | grep amd64

Begin with the last 30 log messages:

    juju debug-log -n 30

Begin with the last 20 log messages for the 'lxd-pilot' model:

    juju debug-log -m lxd-pilot -n 20

Begin with the last 1000 lines and exclude messages from machine 3:

    juju debug-log -n 1000 --exclude machine-3

Select all the messages emitted from a particular unit (you can also write it as
 ` + "`mysql/0`" + `) and a particular machine in the entire log:

juju debug-log --replay --include unit-mysql-0 --include machine-1

View all WARNING and ERROR messages in the entire log:

    juju debug-log --replay --level WARNING

View all WARNING and ERROR messages and then continue showing any
new WARNING and ERROR messages as they are logged:

    juju debug-log --replay --level WARNING

View all logs on the ` + "`cmr`" + ` topic (label):

    juju debug-log --include-label cmr

Progressively exclude more content from the entire log:

    juju debug-log --replay --exclude-module juju.state.apiserver
    juju debug-log --replay --exclude-module juju.state
    juju debug-log --replay --exclude-module juju

Begin with the last 2000 lines and include messages pertaining to both the
` + "`juju.cmd`" + ` and the ` + "`juju.worker modules`" + `:

    juju debug-log --lines 2000 \
        --include-module juju.cmd \
        --include-module juju.worker

Exclude all messages from machine 0 ; show a maximum of 100 lines; and continue to
append filtered messages:

    juju debug-log --exclude machine-0 --lines 100

Include only messages from the ` + "`mysql/0`" + ` unit; show a maximum of 50 lines; and then
exit:

    juju debug-log --include mysql/0 --limit 50

Show all messages from the ` + "`apache/2`" + ` unit or machine 1 and then exit:

    juju debug-log --replay --include apache/2 --include machine-1 --no-tail

Show all ` + "`juju.worker.uniter`" + ` logging module messages that are also unit
` + "`wordpress/0`" + ` messages, and then show any new log messages which match the
filter and append:

    juju debug-log --replay
        --include-module juju.worker.uniter \
        --include wordpress/0

Show all messages from the ` + "`juju.worker.uniter`" + ` module, except those sent from
` + "`machine-3`" + ` or ` + "`machine-4`" + `, and then stop:

    juju debug-log --replay --no-tail
        --include-module juju.worker.uniter \
        --exclude machine-3 \
        --exclude machine-4

`

func (c *debugLogCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "debug-log",
		Purpose:  usageDebugLogSummary,
		Doc:      usageDebugLogDetails,
		Examples: usageDebugLogExamples,
		SeeAlso: []string{
			"status",
			"ssh",
		},
	})
}

func newDebugLogCommand(store jujuclient.ClientStore) cmd.Command {
	return newDebugLogCommandTZ(store, time.Local)
}

func newDebugLogCommandTZ(store jujuclient.ClientStore, tz *time.Location) cmd.Command {
	cmd := &debugLogCommand{tz: tz}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

type debugLogCommand struct {
	modelcmd.ModelCommandBase

	level  string
	params common.DebugLogParams

	utc      bool
	location bool
	date     bool
	ms       bool

	tail        bool
	noTail      bool
	color       bool
	backLogFlag *intValue
	limitFlag   *intValue

	retry      bool
	retryDelay time.Duration

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
	f.Var(cmd.NewAppendStringsValue(&c.params.IncludeLabel), "include-label", "Only show log messages for these logging labels")
	f.Var(cmd.NewAppendStringsValue(&c.params.ExcludeLabel), "exclude-label", "Do not show log messages for these logging labels")

	f.StringVar(&c.level, "l", "", "Log level to show, one of [TRACE, DEBUG, INFO, WARNING, ERROR]")
	f.StringVar(&c.level, "level", "", "")

	c.backLogFlag = newIntValue(&c.params.Backlog)
	f.Var(c.backLogFlag, "n", "Show this many of the most recent lines and continue to append new ones")
	f.Var(c.backLogFlag, "lines", "")

	c.limitFlag = newIntValue(&c.params.Limit)
	f.Var(c.limitFlag, "limit", "Show this many of the most recent logs and then exit")

	f.BoolVar(&c.params.Replay, "replay", false, "Show the entire log and continue to append new ones")

	f.BoolVar(&c.noTail, "no-tail", false, "Show existing log messages and then exit")
	f.BoolVar(&c.tail, "tail", false, "Show existing log messages and continue to append new ones")
	f.BoolVar(&c.color, "color", false, "Force use of ANSI color codes")

	f.BoolVar(&c.utc, "utc", false, "Show times in UTC")
	f.BoolVar(&c.location, "location", false, "Show filename and line numbers")
	f.BoolVar(&c.date, "date", false, "Show dates as well as times")
	f.BoolVar(&c.ms, "ms", false, "Show times to millisecond precision")

	f.BoolVar(&c.retry, "retry", false, "Retry connection on failure")
	f.DurationVar(&c.retryDelay, "retry-delay", 1*time.Second, "Retry delay between connection failure retries")
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
	if c.tail && c.noTail {
		return errors.NotValidf("setting --tail and --no-tail")
	}
	if c.noTail && c.retry {
		return errors.NotValidf("setting --no-tail and --retry")
	}
	if c.noTail && c.backLogFlag.IsSet() {
		return errors.NotValidf("setting --no-tail and --lines")
	}
	if c.tail && c.limitFlag.IsSet() {
		return errors.NotValidf("setting --tail and --limit")
	}
	if c.limitFlag.IsSet() && c.backLogFlag.IsSet() {
		return errors.NotValidf("setting --limit and --lines")
	}
	if c.params.Replay && c.backLogFlag.IsSet() {
		return errors.NotValidf("setting --replay and --lines")
	}
	if c.retryDelay < 0 {
		return errors.NotValidf("negative retry delay")
	}
	if c.limitFlag.IsSet() {
		c.noTail = true
	}
	if c.backLogFlag.IsSet() {
		c.tail = true
	}
	if !c.backLogFlag.IsSet() && !c.limitFlag.IsSet() && !c.params.Replay {
		*c.backLogFlag.value = defaultLineCount
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
	c.params.IncludeEntity = transform.Slice(c.params.IncludeEntity, c.parseEntity)
	c.params.ExcludeEntity = transform.Slice(c.params.ExcludeEntity, c.parseEntity)
	return cmd.CheckEmpty(args)
}

func (c *debugLogCommand) parseEntity(entity string) string {
	tag, err := names.ParseTag(entity)
	switch {
	case strings.Contains(entity, "*"):
		return entity
	case err == nil && (tag.Kind() == names.ApplicationTagKind || tag.Kind() == names.MachineTagKind || tag.Kind() == names.UnitTagKind):
		return tag.String()
	case names.IsValidMachine(entity):
		return names.NewMachineTag(entity).String()
	case names.IsValidUnit(entity):
		return names.NewUnitTag(entity).String()
	case names.IsValidApplication(entity):
		// If the user asks for --include nova-compute, we should give all
		// nova-compute units for IAAS models.
		return names.UnitTagKind + "-" + entity + "-*"
	default:
		logger.Warningf("%q was not recognised as a valid application, machine or unit name", entity)
		return entity
	}
}

type DebugLogAPI interface {
	WatchDebugLog(params common.DebugLogParams) (<-chan common.LogMessage, error)
	Close() error
}

var getDebugLogAPI = func(c *debugLogCommand) (DebugLogAPI, error) {
	return c.NewAPIClient()
}

func isTerminal(f interface{}) bool {
	f_, ok := f.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f_.Fd())
}

// Run retrieves the debug log via the API.
func (c *debugLogCommand) Run(ctx *cmd.Context) error {
	if c.tail {
		c.params.NoTail = false
	} else if c.noTail {
		c.params.NoTail = true
	} else {
		// Set the default tail option to true if the caller is
		// using a terminal.
		c.params.NoTail = !isTerminal(ctx.Stdout)
	}

	writer := ansiterm.NewWriter(ctx.Stdout)
	if c.color {
		writer.SetColorCapable(true)
	}

	err := retry.Call(retry.CallArgs{
		Func: func() error {
			client, err := getDebugLogAPI(c)
			if err != nil {
				return err
			}
			defer client.Close()

			messages, err := client.WatchDebugLog(c.params)
			if err != nil {
				return err
			}

			for {
				msg, ok := <-messages
				if !ok {
					return ErrConnectionClosed
				}
				c.writeLogRecord(writer, msg)
			}
		},
		IsFatalError: func(err error) bool {
			if !c.retry {
				return true
			}
			if errors.Is(err, ErrConnectionClosed) {
				return false
			}
			return true
		},
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf("retrying to connect to debug log")
		},
		Attempts: -1,
		Clock:    clock.WallClock,
		Delay:    c.retryDelay,
		Stop:     ctx.Done(),
	})

	// Ensure that any sentinel ErrConnectionClosed errors are not shown to the
	// user. As this is a synthetic error that is used to signal that the
	// connection is retried, we don't want to show this to the user.
	if errors.Is(err, ErrConnectionClosed) {
		return nil
	}

	// Unwrap the retry call error trace for all errors. We don't want to show
	// that to the user as part of the error message.
	return errors.Cause(err)
}

// ErrConnectionClosed is a sentinel error used to signal that the connection
// is closed.
var ErrConnectionClosed = errors.ConstError("connection closed")

var SeverityColor = map[string]*ansiterm.Context{
	"TRACE":   ansiterm.Foreground(ansiterm.Default),
	"DEBUG":   ansiterm.Foreground(ansiterm.Green),
	"INFO":    ansiterm.Foreground(ansiterm.BrightBlue),
	"WARNING": ansiterm.Foreground(ansiterm.Yellow),
	"ERROR":   ansiterm.Foreground(ansiterm.BrightRed),
	"CRITICAL": {
		Foreground: ansiterm.White,
		Background: ansiterm.Red,
	},
}

func (c *debugLogCommand) writeLogRecord(w *ansiterm.Writer, r common.LogMessage) {
	ts := r.Timestamp.In(c.tz).Format(c.format)
	fmt.Fprintf(w, "%s: %s ", r.Entity, ts)
	SeverityColor[r.Severity].Fprint(w, r.Severity)
	fmt.Fprintf(w, " %s ", r.Module)
	if c.location {
		loggocolor.LocationColor.Fprintf(w, "%s ", r.Location)
	}
	if len(r.Labels) > 0 {
		fmt.Fprintf(w, "%v ", strings.Join(r.Labels, ","))
	}
	fmt.Fprintln(w, r.Message)
}

// intValue implements gnuflag.Value for an int value that can be set
// to differentiate user input value from default value.
type intValue struct {
	value *uint
	isSet bool
}

func newIntValue(val *uint) *intValue {
	return &intValue{
		value: val,
	}
}

// Implements gnuflag.Value Set.
func (v *intValue) Set(s string) error {
	val, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid value %q for uint flag", s)
	}
	// Check if val > max value of uint
	if val > uint64(^uint(0)) {
		return fmt.Errorf("value %q exceeds maximum value", s)
	}
	*v.value = uint(val) // Convert to uint
	v.isSet = true
	return nil
}

// Implements gnuflag.Value String.
func (v *intValue) String() string {
	if v == nil || v.value == nil {
		return ""
	}
	return fmt.Sprint(*v.value)
}

// Returns true if the value has been set.
func (v *intValue) IsSet() bool {
	return v.isSet
}
