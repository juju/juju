// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/juju/ansiterm"
	"github.com/juju/clock"
	"github.com/juju/cmd/v4"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/loggocolor"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/mattn/go-isatty"

	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/highavailability"
	"github.com/juju/juju/api/common"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	corelogger "github.com/juju/juju/core/logger"
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

The "entity" is the source of the message: a machine or unit. The names for
machines and units can be seen in the output of `[1:] + "`juju status`" + `.

The '--include' and '--exclude' options filter by entity. The entity can be
a machine, unit, or application for vm models, but can be application only
for k8s models. These filters support wildcards ` + "`*`" + ` if filtering on the
entity full name (prefixed by ` + "`<entity type>-`" + `)

The '--include-module' and '--exclude-module' options filter by (dotted)
logging module name. The module name can be truncated such that all loggers
with the prefix will match.

The '--include-labels' and '--exclude-labels' options filter by logging labels. 

The filtering options combine as follows:
* All --include options are logically ORed together.
* All --exclude options are logically ORed together.
* All --include-module options are logically ORed together.
* All --exclude-module options are logically ORed together.
* All --include-labels options are logically ORed together.
* All --exclude-labels options are logically ORed together.
* The combined --include, --exclude, --include-module, --exclude-module,
  --include-labels and --exclude-labels selections are logically ANDed to form
  the complete filter.

`

const usageDebugLogExamples = `
Exclude all machine 0 messages; show a maximum of 100 lines; and continue to
append filtered messages:

    juju debug-log --exclude machine-0 --lines 100

Include only messages from the mysql/0 unit; show a maximum of 50 lines; and then
exit:

    juju debug-log --include mysql/0 --limit 50

Show all messages from the apache/2 unit or machine 1 and then exit:

    juju debug-log --replay --include apache/2 --include machine-1 --no-tail

Show all juju.worker.uniter logging module messages that are also unit
wordpress/0 messages, and then show any new log messages which match the
filter and append:

    juju debug-log --replay
        --include-module juju.worker.uniter \
        --include wordpress/0

Show all messages from the juju.worker.uniter module, except those sent from
machine-3 or machine-4, and then stop:

    juju debug-log --replay --no-tail
        --include-module juju.worker.uniter \
        --exclude machine-3 \
        --exclude machine-4

To see all WARNING and ERROR messages and then continue showing any
new WARNING and ERROR messages as they are logged:

    juju debug-log --replay --level WARNING

In the HA case, debug-log can be configured to stream messages from a selected controller.
Use juju show-controller to see the available controller numbers.

    juju debug-log --controller 2

You can also stream messages from all controllers - a best effort will be made to correctly
interleave them so they are ordered by timestamp.

    juju debug-log --controller all
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

	tw  *ansiterm.Writer
	out cmd.Output

	level  string
	params common.DebugLogParams

	utc      bool
	location bool
	date     bool
	ms       bool

	tail   bool
	noTail bool
	color  bool

	retry      bool
	retryDelay time.Duration

	format string
	tz     *time.Location

	includeLabels []string
	excludeLabels []string

	controllerIdOrAll string
}

func (c *debugLogCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.Var(cmd.NewAppendStringsValue(&c.params.IncludeEntity), "i", "Only show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.IncludeEntity), "include", "Only show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.ExcludeEntity), "x", "Do not show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.ExcludeEntity), "exclude", "Do not show log messages for these entities")
	f.Var(cmd.NewAppendStringsValue(&c.params.IncludeModule), "include-module", "Only show log messages for these logging modules")
	f.Var(cmd.NewAppendStringsValue(&c.params.ExcludeModule), "exclude-module", "Do not show log messages for these logging modules")
	f.Var(cmd.NewAppendStringsValue(&c.includeLabels), "include-labels", "Only show log messages for these logging label key values")
	f.Var(cmd.NewAppendStringsValue(&c.excludeLabels), "exclude-labels", "Do not show log messages for these logging label key values")

	f.StringVar(&c.controllerIdOrAll, "controller", "", "A specific controller from which to display logs, or 'all' for interleaved logs from all controllers.")

	f.StringVar(&c.level, "l", "", "Log level to show, one of [TRACE, DEBUG, INFO, WARNING, ERROR]")
	f.StringVar(&c.level, "level", "", "")

	f.UintVar(&c.params.Backlog, "n", defaultLineCount, "Show this many of the most recent (possibly filtered) lines, and continue to append")
	f.UintVar(&c.params.Backlog, "lines", defaultLineCount, "")
	f.UintVar(&c.params.Limit, "limit", 0, "Exit once this many of the most recent (possibly filtered) lines are shown")
	f.BoolVar(&c.params.Replay, "replay", false, "Show the entire (possibly filtered) log and continue to append")

	f.BoolVar(&c.noTail, "no-tail", false, "Stop after returning existing log messages")
	f.BoolVar(&c.tail, "tail", false, "Wait for new logs")
	f.BoolVar(&c.color, "color", false, "Force use of ANSI color codes")

	f.BoolVar(&c.utc, "utc", false, "Show times in UTC")
	f.BoolVar(&c.location, "location", false, "Show filename and line numbers")
	f.BoolVar(&c.date, "date", false, "Show dates as well as times")
	f.BoolVar(&c.ms, "ms", false, "Show times to millisecond precision")

	f.BoolVar(&c.retry, "retry", false, "Retry connection on failure")
	f.DurationVar(&c.retryDelay, "retry-delay", 1*time.Second, "Retry delay between connection failure retries")

	c.out.AddFlags(f, "text", map[string]cmd.Formatter{
		"json": cmd.FormatJson,
		"text": c.writeText,
	})
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
	if c.retryDelay < 0 {
		return errors.NotValidf("negative retry delay")
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

	for _, label := range c.includeLabels {
		parts := strings.Split(label, "=")
		if len(parts) < 2 {
			return fmt.Errorf("include label key value %q %w", label, errors.NotValid)
		}
		if c.params.IncludeLabels == nil {
			c.params.IncludeLabels = make(map[string]string)
		}
		c.params.IncludeLabels[parts[0]] = parts[1]
	}
	for _, label := range c.excludeLabels {
		parts := strings.Split(label, "=")
		if len(parts) < 2 {
			return fmt.Errorf("exclude label key value %q %w", label, errors.NotValid)
		}
		if c.params.ExcludeLabels == nil {
			c.params.ExcludeLabels = make(map[string]string)
		}
		c.params.ExcludeLabels[parts[0]] = parts[1]
	}

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

// DebugLogAPI provides access to the client facade.
type DebugLogAPI interface {
	WatchDebugLog(params common.DebugLogParams) (<-chan common.LogMessage, error)
	Close() error
}

// ControllerDetailsAPI provides access to the high availability facade.
type ControllerDetailsAPI interface {
	ControllerDetails() (map[string]highavailability.ControllerDetails, error)
	BestAPIVersion() int
}

var getDebugLogAPI = func(c *debugLogCommand, addr []string) (DebugLogAPI, error) {
	root, err := c.NewAPIRootWithAddressOverride(addr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiclient.NewClient(root, logger), nil
}

var getControllerDetailsClient = func(c *debugLogCommand) (ControllerDetailsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return highavailability.NewClient(root), nil
}

func isTerminal(f interface{}) bool {
	f_, ok := f.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f_.Fd())
}

type logFunc func([]corelogger.LogRecord) error

func (f logFunc) Log(r []corelogger.LogRecord) error {
	return f(r)
}

func (c *debugLogCommand) getControllerAddresses() ([][]string, error) {
	if c.controllerIdOrAll == "" {
		return nil, nil
	}

	api, err := getControllerDetailsClient(c)
	if err != nil {
		return nil, errors.Annotate(err, "getting controller HA client")
	}
	if api.BestAPIVersion() < 3 {
		return nil, fmt.Errorf("debug log controller selection not supported with this version of Juju")
	}
	controllerDetails, err := api.ControllerDetails()
	if err != nil {
		return nil, errors.Annotate(err, "getting controller details")
	}

	var controllerAddr [][]string
	if c.controllerIdOrAll == "all" {
		for _, ctrl := range controllerDetails {
			controllerAddr = append(controllerAddr, ctrl.APIEndpoints)
		}

	} else {
		ctrl, ok := controllerDetails[c.controllerIdOrAll]
		if !ok {
			return nil, fmt.Errorf("controller %q %w", c.controllerIdOrAll, errors.NotFound)
		}
		controllerAddr = append(controllerAddr, ctrl.APIEndpoints)
	}
	return controllerAddr, nil
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

	// Get the controller addresses to connect to.
	controllerAddr, err := c.getControllerAddresses()
	if err != nil {
		return err
	}

	// The default log buffer size is 1 for a single controller
	// (stream log entries as they arrive).
	bufferSize := 1
	// If we are connecting to multiple controllers, adjust the buffer size so that
	// we have a chance of ordering incoming records by timestamp.
	// Replaying the logs will likely stream many initially so use a larger buffer size
	// to account for controllers having potentially divergent log entry timestamps.
	// This is best effort so it's ok if some log entries are printed out of order.
	// Ideally we'd have a variable flush timeout depending on the rate of incoming messages
	// but the buffered logger doesn't support that.
	if len(controllerAddr) > 1 {
		if c.params.Replay || c.params.NoTail {
			bufferSize = 500
		} else {
			bufferSize = 5
		}
	}

	buf := corelogger.NewBufferedLogger(logFunc(func(recs []corelogger.LogRecord) error {
		for _, r := range recs {
			_ = c.out.Write(ctx, &r)
		}
		return nil
	}), bufferSize, time.Second, clock.WallClock)

	// Start one log streamer per controller.
	numStreams := 1
	if n := len(controllerAddr); n > 0 {
		numStreams = n
	}
	// Size of errors channel and wait group needs to match the number of debug log streams.
	var (
		errs = make(chan error, numStreams)
		wg   sync.WaitGroup
	)
	wg.Add(numStreams)

	pollCtx, cancel := context.WithCancel(ctx)
	if len(controllerAddr) == 0 {
		go func() {
			c.streamLogs(pollCtx, nil, buf, errs)
			wg.Done()
		}()
	} else {
		for _, addr := range controllerAddr {
			go func(addr []string) {
				c.streamLogs(pollCtx, addr, buf, errs)
				wg.Done()
			}(addr)
		}
	}

	// Wait for the streams to exit.
	var pollErr error
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case pollErr = <-errs:
			// Exit on the first error.
			break loop
		}
	}
	// Cancel the message polling before flushing the buffer.
	cancel()
	wg.Wait()
	_ = buf.Flush()
	return pollErr
}

// streamLogs watches debug logs from the specified controller and logs any results
// into the supplied buffered logger. Any error is reported to the errors channel.
func (c *debugLogCommand) streamLogs(ctx context.Context, controllerAddr []string, buf *corelogger.BufferedLogger, errs chan error) {
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			client, err := getDebugLogAPI(c, controllerAddr)
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
				level, _ := loggo.ParseLevel(msg.Severity)
				logRecord := corelogger.LogRecord{
					Time:     msg.Timestamp,
					Entity:   msg.Entity,
					Level:    level,
					Module:   msg.Module,
					Location: msg.Location,
					Message:  msg.Message,
					Labels:   msg.Labels,
				}
				if err := buf.Log([]corelogger.LogRecord{logRecord}); err != nil {
					return err
				}
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
		err = nil
	}
	// Unwrap the retry call error trace for all errors. We don't want to show
	// that to the user as part of the error message.
	errs <- errors.Cause(err)
}

// ErrConnectionClosed is a sentinel error used to signal that the connection
// is closed.
var ErrConnectionClosed = errors.ConstError("connection closed")

var SeverityColor = map[loggo.Level]*ansiterm.Context{
	loggo.TRACE:   ansiterm.Foreground(ansiterm.Default),
	loggo.DEBUG:   ansiterm.Foreground(ansiterm.Green),
	loggo.INFO:    ansiterm.Foreground(ansiterm.BrightBlue),
	loggo.WARNING: ansiterm.Foreground(ansiterm.Yellow),
	loggo.ERROR:   ansiterm.Foreground(ansiterm.BrightRed),
	loggo.CRITICAL: {
		Foreground: ansiterm.White,
		Background: ansiterm.Red,
	},
}

func (c *debugLogCommand) writeText(w io.Writer, v interface{}) error {
	if c.tw == nil {
		c.tw = ansiterm.NewWriter(w)
		if c.color {
			c.tw.SetColorCapable(true)
		}
	}
	r, ok := v.(*corelogger.LogRecord)
	if !ok {
		return fmt.Errorf("expected log message of type %T, got %t", common.LogMessage{}, v)
	}
	ts := r.Time.In(c.tz).Format(c.format)
	fmt.Fprintf(w, "%s: %s ", r.Entity, ts)
	SeverityColor[r.Level].Fprintf(c.tw, r.Level.String())
	fmt.Fprintf(w, " %s ", r.Module)
	if c.location {
		loggocolor.LocationColor.Fprintf(c.tw, "%s ", r.Location)
	}
	if len(r.Labels) > 0 {
		var labelsOut []string
		for k, v := range r.Labels {
			labelsOut = append(labelsOut, fmt.Sprintf("%s:%s", k, v))
		}
		fmt.Fprintf(w, "%v ", strings.Join(labelsOut, ","))
	}
	fmt.Fprintln(c.tw, r.Message)
	return nil
}
