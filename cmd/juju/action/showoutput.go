// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/watcher"
)

func NewShowActionOutputCommand() cmd.Command {
	return modelcmd.Wrap(&showOutputCommand{
		compat: true,
		logMessageHandler: func(ctx *cmd.Context, msg string) {
			fmt.Fprintln(ctx.Stderr, msg)
		},
	})
}

func NewShowTaskCommand() cmd.Command {
	return modelcmd.Wrap(&showOutputCommand{
		compat: false,
		logMessageHandler: func(ctx *cmd.Context, msg string) {
			fmt.Fprintln(ctx.Stderr, msg)
		},
	})
}

// showOutputCommand fetches the results of an action by ID.
type showOutputCommand struct {
	ActionCommandBase
	out         cmd.Output
	requestedId string
	fullSchema  bool
	// TODO(juju3) - remove legacyWait
	legacyWait string
	wait       time.Duration
	watch      bool
	utc        bool

	// compat is true when running as legacy show-action-output
	compat bool

	logMessageHandler func(*cmd.Context, string)
}

const showOutputDoc = `
Show the results returned by an action with the given ID.  
To block until the result is known completed or failed, use
the --wait option with a duration, as in --wait 5s or --wait 1h.
Use --watch to wait indefinitely.  

The default behavior without --wait or --watch is to immediately check and return;
if the results are "pending" then only the available information will be
displayed.  This is also the behavior when any negative time is given.

Note: if Juju has been upgraded from 2.6 and there are old action UUIDs still in use,
and you want to specify just the UUID prefix to match on, you will need to include up
to at least the first "-" to disambiguate from a newer numeric id.

Examples:

    juju show-action-output 1
    juju show-action-output 1 --wait=2m
    juju show-action-output 1 --watch

See also:
    run-action
    list-operations
    show-operation
`
const defaultTaskWait = -1 * time.Second

// Set up the output.
func (c *showOutputCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	defaultFormatter := "yaml"
	if !c.compat {
		defaultFormatter = "plain"
	}
	c.out.AddFlags(f, defaultFormatter, map[string]cmd.Formatter{
		"yaml":  cmd.FormatYaml,
		"json":  cmd.FormatJson,
		"plain": printPlainOutput,
	})

	if c.compat {
		f.StringVar(&c.legacyWait, "wait", "-1s", "Wait for results")
	} else {
		f.DurationVar(&c.wait, "wait", defaultTaskWait, "Wait for results")
	}
	f.BoolVar(&c.watch, "watch", false, "Wait indefinitely for results")
	f.BoolVar(&c.utc, "utc", false, "Show times in UTC")
}

func (c *showOutputCommand) Info() *cmd.Info {
	info := jujucmd.Info(&cmd.Info{
		Name:    "show-action-output",
		Args:    "<action>",
		Purpose: "Show results of an action.",
		Doc:     showOutputDoc,
	})
	if !c.compat {
		info.Name = "show-task"
		info.Args = "<task ID>"
		info.Purpose = "Show results of a task by ID."
		info.Doc = strings.Replace(info.Doc, "an action", "a task", -1)
		info.Doc = strings.Replace(info.Doc, "show-action-output", "show-task", -1)
		info.Doc = strings.Replace(info.Doc, "run-action", "run", -1)
	}
	return info
}

// Init validates the action ID and any other options.
func (c *showOutputCommand) Init(args []string) error {
	if c.compat {
		// Check whether units were left off our time string.
		r := regexp.MustCompile("[a-zA-Z]")
		matches := r.FindStringSubmatch(c.legacyWait[len(c.legacyWait)-1:])
		// If any match, we have units.  Otherwise, we don't; assume seconds.
		if len(matches) == 0 {
			c.legacyWait = c.legacyWait + "s"
		}

		waitDur, err := time.ParseDuration(c.legacyWait)
		if err != nil {
			return err
		}
		c.wait = waitDur
	}

	if c.watch {
		if c.wait != defaultTaskWait {
			return errors.New("specify either --watch or --wait but not both")
		}
		// If we are watching the wait is 0 (indefinite).
		c.wait = 0 * time.Second
	}
	switch len(args) {
	case 0:
		if c.compat {
			return errors.New("no action ID specified")
		}
		return errors.New("no task ID specified")
	case 1:
		c.requestedId = args[0]
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

// Run issues the API call to get Actions by ID.
func (c *showOutputCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	wait := time.NewTimer(c.wait)
	if c.wait.Nanoseconds() == 0 {
		// Zero duration signals indefinite wait.  Discard the tick.
		_ = <-wait.C
	}

	actionDone := make(chan struct{})
	var logsWatcher watcher.StringsWatcher
	haveLogs := false

	shouldWatch := c.wait.Nanoseconds() >= 0
	if shouldWatch {
		result, err := fetchResult(api, c.requestedId, c.compat)
		if err != nil {
			return errors.Trace(err)
		}
		shouldWatch = result.Status == params.ActionPending ||
			result.Status == params.ActionRunning
	}

	if shouldWatch && api.BestAPIVersion() >= 5 {
		logsWatcher, err = api.WatchActionProgress(c.requestedId)
		if err != nil {
			return errors.Trace(err)
		}
		processLogMessages(logsWatcher, actionDone, ctx, c.utc, func(ctx *cmd.Context, msg string) {
			haveLogs = true
			c.logMessageHandler(ctx, msg)
		})
	}

	var result params.ActionResult
	if shouldWatch {
		result, err = GetActionResult(api, c.requestedId, wait, c.compat)
	} else {
		result, err = fetchResult(api, c.requestedId, c.compat)
	}
	close(actionDone)
	if logsWatcher != nil {
		logsWatcher.Wait()
	}
	if haveLogs {
		// Make the logs a bit separate in the output.
		fmt.Fprintln(ctx.Stderr, "")
	}
	if err != nil {
		return errors.Trace(err)
	}

	formatted := FormatActionResult(c.requestedId, result, c.utc, c.compat)
	if c.out.Name() != "plain" {
		return c.out.Write(ctx, formatted)
	}
	info := make(map[string]interface{})
	info[c.requestedId] = formatted
	return c.out.Write(ctx, info)
}

// GetActionResult tries to repeatedly fetch an action until it is
// in a completed state and then it returns it.
// It waits for a maximum of "wait" before returning with the latest action status.
func GetActionResult(api APIClient, requestedId string, wait *time.Timer, compat bool) (params.ActionResult, error) {

	// tick every two seconds, to delay the loop timer.
	// TODO(fwereade): 2016-03-17 lp:1558657
	tick := time.NewTimer(2 * time.Second)

	return timerLoop(api, requestedId, wait, tick, compat)
}

// timerLoop loops indefinitely to query the given API, until "wait" times
// out, using the "tick" timer to delay the API queries.  It writes the
// result to the given output.
func timerLoop(api APIClient, requestedId string, wait, tick *time.Timer, compat bool) (params.ActionResult, error) {
	var (
		result params.ActionResult
		err    error
	)

	// Loop over results until we get "failed" or "completed".  Wait for
	// timer, and reset it each time.
	for {
		result, err = fetchResult(api, requestedId, compat)
		if err != nil {
			return result, err
		}

		// Whether or not we're waiting for a result, if a completed
		// result arrives, we're done.
		switch result.Status {
		case params.ActionRunning, params.ActionPending:
		default:
			return result, nil
		}

		// Block until a tick happens, or the timeout arrives.
		select {
		case _ = <-wait.C:
			switch result.Status {
			case params.ActionRunning, params.ActionPending:
				return result, errors.NewTimeout(err, "timeout reached")
			default:
				return result, nil
			}
		case _ = <-tick.C:
			tick.Reset(2 * time.Second)
		}
	}
}

// fetchResult queries the given API for the given Action ID prefix, and
// makes sure the results are acceptable, returning an error if they are not.
func fetchResult(api APIClient, requestedId string, compat bool) (params.ActionResult, error) {
	none := params.ActionResult{}

	actionTag, err := getActionTagByPrefix(api, requestedId)
	if err != nil {
		return none, err
	}

	actions, err := api.Actions(params.Entities{
		Entities: []params.Entity{{actionTag.String()}},
	})
	if err != nil {
		return none, err
	}
	actionResults := actions.Results
	numActionResults := len(actionResults)
	task := "task"
	if compat {
		task = "action"
	}
	if numActionResults == 0 {
		return none, errors.Errorf("no results for %s %s", task, requestedId)
	}
	if numActionResults != 1 {
		return none, errors.Errorf("too many results for %s %s", task, requestedId)
	}

	result := actionResults[0]
	if result.Error != nil {
		return none, result.Error
	}

	return result, nil
}

// FormatActionResult removes empty values from the given ActionResult and
// inserts the remaining ones in a map[string]interface{} for cmd.Output to
// write in an easy-to-read format.
func FormatActionResult(id string, result params.ActionResult, utc, compat bool) map[string]interface{} {
	response := map[string]interface{}{"id": id, "status": result.Status}
	if result.Action != nil {
		ut, err := names.ParseUnitTag(result.Action.Receiver)
		if err == nil {
			response["unit"] = ut.Id()
		}
	}
	if result.Message != "" {
		response["message"] = result.Message
	}
	if len(result.Output) != 0 {
		if compat {
			output := ConvertActionOutput(result.Output, compat, false)
			response["results"] = output
		} else {
			response["results"] = result.Output
		}
	}
	if len(result.Log) > 0 {
		var logs []string
		for _, msg := range result.Log {
			logs = append(logs, formatLogMessage(actions.ActionMessage{
				Timestamp: msg.Timestamp,
				Message:   msg.Message,
			}, false, utc, false))
		}
		response["log"] = logs
	}

	if unit, ok := response["UnitId"]; ok && !compat {
		delete(response, "UnitId")
		response["unit"] = unit
	}
	if unit, ok := response["unit"]; ok && compat {
		delete(response, "unit")
		response["UnitId"] = unit
	}

	if result.Enqueued.IsZero() && result.Started.IsZero() && result.Completed.IsZero() {
		return response
	}

	responseTiming := make(map[string]string)
	for k, v := range map[string]string{
		"enqueued":  formatTimestamp(result.Enqueued, false, utc || compat, false),
		"started":   formatTimestamp(result.Started, false, utc || compat, false),
		"completed": formatTimestamp(result.Completed, false, utc || compat, false),
	} {
		if v != "" {
			responseTiming[k] = v
		}
	}
	response["timing"] = responseTiming

	return response
}

// ConvertActionOutput returns result data with stdout, stderr etc correctly formatted.
func ConvertActionOutput(output map[string]interface{}, compat, alwaysStdout bool) map[string]interface{} {
	if output == nil {
		return nil
	}
	values := output
	// We always want to have a string for stdout, but only show stderr,
	// code and error if they are there.
	stdoutKey := "stdout"
	if compat {
		stdoutKey = "Stdout"
	}
	res, ok := output["Stdout"].(string)
	if !ok {
		res, ok = output["stdout"].(string)
	}
	if ok && len(res) > 0 {
		values[stdoutKey] = strings.Replace(res, "\r\n", "\n", -1)
		if res, ok := output["StdoutEncoding"].(string); ok && res != "" {
			values["Stdout-encoding"] = res
		}
	} else if compat && alwaysStdout {
		values[stdoutKey] = ""
	} else {
		delete(values, "stdout")
	}
	stderrKey := "stderr"
	if compat {
		stderrKey = "Stderr"
	}
	res, ok = output["Stderr"].(string)
	if !ok {
		res, ok = output["stderr"].(string)
	}
	if ok && len(res) > 0 {
		values[stderrKey] = strings.Replace(res, "\r\n", "\n", -1)
		if res, ok := output["StderrEncoding"].(string); ok && res != "" {
			values["Stderr-encoding"] = res
		}
	} else {
		delete(values, "stderr")
	}
	codeKey := "return-code"
	if compat {
		codeKey = "ReturnCode"
	}
	res, ok = output["Code"].(string)
	delete(values, "Code")
	if !ok {
		var v interface{}
		if v, ok = output["return-code"]; ok && v != nil {
			res = fmt.Sprintf("%v", v)
		}
	}
	if ok && len(res) > 0 {
		code, err := strconv.Atoi(res)
		if err == nil && (code != 0 || !compat) {
			values[codeKey] = code
		}
	} else {
		delete(values, "return-code")
	}

	if compat {
		delete(values, "stdout")
		delete(values, "stderr")
		delete(values, "return-code")
		delete(values, "stdout-encoding")
		delete(values, "stderr-encoding")
	}
	return values
}
