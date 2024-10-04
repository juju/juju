// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	actionapi "github.com/juju/juju/api/client/action"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

func NewShowTaskCommand() cmd.Command {
	return modelcmd.Wrap(&showTaskCommand{
		logMessageHandler: func(ctx *cmd.Context, msg string) {
			fmt.Fprintln(ctx.Stderr, msg)
		},
		clock: clock.WallClock,
	})
}

// showTaskCommand fetches the results of a task by ID.
type showTaskCommand struct {
	ActionCommandBase
	out         cmd.Output
	requestedId string
	wait        time.Duration
	watch       bool
	utc         bool

	clock             clock.Clock
	logMessageHandler func(*cmd.Context, string)
}

const showTaskDoc = `
Show the results returned by a task with the given ID.  
To block until the result is known completed or failed, use
the --wait option with a duration, as in --wait 5s or --wait 1h.
Use --watch to wait indefinitely.  

The default behavior without --wait or --watch is to immediately check and return;
if the results are "pending" then only the available information will be
displayed.  This is also the behavior when any negative time is given.

Note: if Juju has been upgraded from 2.6 and there are old action UUIDs still in use,
and you want to specify just the UUID prefix to match on, you will need to include up
to at least the first "-" to disambiguate from a newer numeric id.
`

const showTaskExamples = `
    juju show-task 1
    juju show-task 1 --wait=2m
    juju show-task 1 --watch
`

const defaultTaskWait = -1 * time.Second

// Set up the output.
func (c *showTaskCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	defaultFormatter := "plain"
	c.out.AddFlags(f, defaultFormatter, map[string]cmd.Formatter{
		"yaml":  cmd.FormatYaml,
		"json":  cmd.FormatJson,
		"plain": printOutput,
	})

	f.DurationVar(&c.wait, "wait", defaultTaskWait, "Maximum wait time for a task to complete")
	f.BoolVar(&c.watch, "watch", false, "Wait indefinitely for results")
	f.BoolVar(&c.utc, "utc", false, "Show times in UTC")
}

func (c *showTaskCommand) Info() *cmd.Info {
	info := jujucmd.Info(&cmd.Info{
		Name:     "show-task",
		Args:     "<task ID>",
		Purpose:  "Show results of a task by ID.",
		Doc:      showTaskDoc,
		Examples: showTaskExamples,
		SeeAlso: []string{
			"cancel-task",
			"run",
			"operations",
			"show-operation",
		},
	})
	return info
}

// Init validates the action ID and any other options.
func (c *showTaskCommand) Init(args []string) error {
	if c.watch {
		if c.wait != defaultTaskWait {
			return errors.New("specify either --watch or --wait but not both")
		}
		// If we are watching the wait is 0 (indefinite).
		c.wait = 0 * time.Second
	}
	switch len(args) {
	case 0:
		return errors.New("no task ID specified")
	case 1:
		if !names.IsValidAction(args[0]) {
			return errors.NotValidf("task ID %q", args[0])
		}
		c.requestedId = args[0]
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

// Run issues the API call to get Actions by ID.
func (c *showTaskCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient(ctx)
	if err != nil {
		return err
	}
	defer api.Close()

	wait := c.clock.NewTimer(c.wait)
	if c.wait.Nanoseconds() == 0 {
		// Zero duration signals indefinite wait.  Discard the tick.
		<-wait.Chan()
	}

	actionDone := make(chan struct{})
	var logsWatcher watcher.StringsWatcher
	haveLogs := false

	shouldWatch := c.wait.Nanoseconds() >= 0
	if shouldWatch {
		result, err := fetchResult(ctx, api, c.requestedId)
		if err != nil {
			return errors.Trace(err)
		}
		shouldWatch = result.Status == params.ActionPending ||
			result.Status == params.ActionRunning
	}

	if shouldWatch {
		logsWatcher, err = api.WatchActionProgress(ctx, c.requestedId)
		if err != nil {
			return errors.Trace(err)
		}
		processLogMessages(logsWatcher, actionDone, ctx, c.utc, func(ctx *cmd.Context, msg string) {
			haveLogs = true
			c.logMessageHandler(ctx, msg)
		})
	}

	var result actionapi.ActionResult
	if shouldWatch {
		result, err = GetActionResult(ctx, api, c.requestedId, c.clock, wait)
	} else {
		result, err = fetchResult(ctx, api, c.requestedId)
	}
	close(actionDone)
	if logsWatcher != nil {
		_ = logsWatcher.Wait()
	}
	if haveLogs {
		// Make the logs a bit separate in the output.
		fmt.Fprintln(ctx.Stderr, "")
	}
	if err != nil {
		return errors.Trace(err)
	}

	formatted, _ := formatActionResult(c.requestedId, result, c.utc)
	if c.out.Name() != "plain" {
		return c.out.Write(ctx, formatted)
	}
	info := make(map[string]interface{})
	info[c.requestedId] = formatted
	return c.out.Write(ctx, info)
}
