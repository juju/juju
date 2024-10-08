// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/ansiterm"
	"github.com/juju/clock"
	"github.com/juju/cmd/v4"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"github.com/mattn/go-isatty"
	"gopkg.in/yaml.v2"

	actionapi "github.com/juju/juju/api/client/action"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/output"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/charm"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

var logger = internallogger.GetLogger("juju.cmd.juju.action")

const (
	// leaderSnippet is a regular expression for unit ID-like syntax that is used
	// to indicate the current leader for an application.
	leaderSnippet = "(" + names.ApplicationSnippet + ")/leader"
	// unitOrLeaderSnippet is a regular expression to match either a standard unit
	// unit ID or the unit ID-like syntax for the leader of an application
	unitOrLeaderSnippet = "(" + names.ApplicationSnippet + ")/(" + names.NumberSnippet + "|leader)"
)

var (
	validLeader       = regexp.MustCompile("^" + leaderSnippet + "$")
	validUnitOrLeader = regexp.MustCompile("^" + unitOrLeaderSnippet + "$")

	// nameRule describes the name format of an action or keyName must match to be valid.
	nameRule = charm.GetActionNameRule()

	// resultPollMinTime is how quickly the first update triggers, we then exponentially back off until we hit resultMaxPollTime
	resultPollMinTime = 20 * time.Millisecond
	// resultPollMaxTime is the maximum time we will spend between updates for results
	resultPollMaxTime = 2 * time.Second
	// resultPollBackoffFactor is the ratio between retries
	resultPollBackoffFactor = 1.5
)

type runCommandBase struct {
	ActionCommandBase
	api        APIClient
	background bool
	out        cmd.Output
	utc        bool

	clock             clock.Clock
	color             bool
	noColor           bool
	wait              time.Duration
	defaultWait       time.Duration
	logMessageHandler func(*cmd.Context, string)

	hideProgress bool // whether to hide progress info by default
}

// SetFlags offers an option for YAML output.
func (c *runCommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "plain", map[string]cmd.Formatter{
		"yaml":  c.formatYaml,
		"json":  c.formatJson,
		"plain": c.printRunOutput,
	})
	c.setNonFormatFlags(f)
}

// SetNonFormatFlags sets all flags except the format one. This is needed by
// e.g. exec to define its own formatting flags.
func (c *runCommandBase) setNonFormatFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	f.BoolVar(&c.background, "background", false, "Run the task in the background")
	f.DurationVar(&c.wait, "wait", 0, "Maximum wait time for a task to complete")
	f.BoolVar(&c.noColor, "no-color", false, "Disable ANSI color codes in output")
	f.BoolVar(&c.color, "color", false, "Use ANSI color codes in output")
	f.BoolVar(&c.utc, "utc", false, "Show times in UTC")
}

func (c *runCommandBase) Init(_ []string) error {
	if c.background && c.wait > 0 {
		return errors.New("cannot specify both --wait and --background")
	}
	if !c.background && c.wait == 0 {
		c.wait = c.defaultWait
		if c.wait == 0 {
			c.wait = 60 * time.Second
		}
	}

	return nil
}

func (c *runCommandBase) ensureAPI(ctx context.Context) (err error) {
	if c.api != nil {
		return nil
	}
	c.api, err = c.NewActionAPIClient(ctx)
	return errors.Trace(err)
}

func (c *runCommandBase) operationResults(ctx *cmd.Context, results *actionapi.EnqueuedActions) error {

	if _, ok := os.LookupEnv("NO_COLOR"); (ok || os.Getenv("TERM") == "dumb") && !c.color || c.noColor {
		return c.processOperationResults(ctx, false, results)
	}

	if c.color {
		return c.processOperationResults(ctx, true, results)
	}

	if isTerminal(ctx.Stdout) && !c.noColor {
		return c.processOperationResults(ctx, true, results)
	}

	if !isTerminal(ctx.Stdout) && c.color {
		return c.processOperationResults(ctx, true, results)
	}

	return c.processOperationResults(ctx, false, results)
}

func (c *runCommandBase) processOperationResults(ctx *cmd.Context, forceColor bool, results *actionapi.EnqueuedActions) error {
	var runningTasks []enqueuedAction
	var enqueueErrs []string
	for _, a := range results.Actions {
		if a.Error != nil {
			enqueueErrs = append(enqueueErrs, a.Error.Error())
			continue
		}
		runningTasks = append(runningTasks, enqueuedAction{
			task:     a.Action.ID,
			receiver: a.Action.Receiver,
		})
	}
	operationID := results.OperationID
	numTasks := len(runningTasks)
	opIDColored := colorVal(output.InfoHighlight, operationID)
	numTasksColored := colorVal(output.EmphasisHighlight.Magenta, numTasks)
	if !c.background && numTasks > 0 {
		var plural string
		if numTasks > 1 {
			plural = "s"
		}
		if forceColor {
			c.progressf(ctx, "Running operation %s with %s task%s", opIDColored, numTasksColored, plural)
		} else {
			c.progressf(ctx, "Running operation %s with %d task%s", operationID, numTasks, plural)
		}
	}

	var actionID string
	info := make(map[string]interface{}, numTasks)
	for _, result := range runningTasks {
		actionID = result.task

		if !c.background {
			if forceColor {
				c.progressf(ctx, "  - task %s on %s", colorVal(output.InfoHighlight, actionID), colorVal(output.EmphasisHighlight.DefaultBold, result.receiver))
			} else {
				c.progressf(ctx, "  - task %s on %s", actionID, result.receiver)
			}
		}
		info[result.receiverId()] = map[string]string{
			"id": result.task,
		}
	}
	if !c.background {
		c.progressf(ctx, "")
	}
	if numTasks == 0 {
		if forceColor {
			ctx.Infof("Operation %s failed to schedule any tasks:\n%s", opIDColored, colorVal(output.ErrorHighlight, strings.Join(enqueueErrs, "\n")))
		} else {
			ctx.Infof("Operation %s failed to schedule any tasks:\n%s", operationID, strings.Join(enqueueErrs, "\n"))
		}
		return nil
	}
	if len(enqueueErrs) > 0 {
		if forceColor {
			ctx.Infof("Some actions could not be scheduled:\n%s\n", colorVal(output.ErrorHighlight, strings.Join(enqueueErrs, "\n")))
		} else {
			ctx.Infof("Some actions could not be scheduled:\n%s\n", strings.Join(enqueueErrs, "\n"))
		}
		ctx.Infof("")
	}
	printInfo := func(opID, actionId, nTasks interface{}) {
		if numTasks == 1 {
			ctx.Infof("Scheduled operation %s with task %s", opID, actionId)
			ctx.Infof("Check operation status with 'juju show-operation %s'", opID)
			ctx.Infof("Check task status with 'juju show-task %s'", actionId)
		} else {
			ctx.Infof("Scheduled operation %s with %v tasks", opID, nTasks)
			if forceColor {
				_ = output.FormatYamlWithColor(ctx.Stdout, info)
			} else {
				_ = cmd.FormatYaml(ctx.Stdout, info)
			}
			ctx.Infof("Check operation status with 'juju show-operation %s'", opID)
			ctx.Infof("Check task status with 'juju show-task <id>'")
		}
	}
	actionIdColored := colorVal(output.InfoHighlight, actionID)
	if c.background {
		if forceColor {
			printInfo(opIDColored, actionIdColored, numTasksColored)
		} else {
			printInfo(operationID, actionID, numTasks)
		}
		return nil
	}
	failed, err := c.waitForTasks(ctx, runningTasks, info)
	if err != nil {
		return errors.Trace(err)
	} else if len(failed) > 0 {
		var plural string
		if len(failed) > 1 {
			plural = "s"
		}
		list := make([]string, 0, len(failed))
		for k, v := range failed {
			if forceColor {
				list = append(list, fmt.Sprintf(" - id %q with return code %v", k, colorVal(output.EmphasisHighlight.Magenta, v)))
			} else {
				list = append(list, fmt.Sprintf(" - id %q with return code %d", k, v))
			}
		}
		sort.Strings(list)

		return errors.Errorf(`
the following task%s failed:
%s

use 'juju show-task' to inspect the failure%s
`[1:], plural, strings.Join(list, "\n"), plural)
	}
	return nil
}

func (c *runCommandBase) waitForTasks(ctx *cmd.Context, runningTasks []enqueuedAction, info map[string]interface{}) (map[string]int, error) {
	var wait clock.Timer
	if c.wait < 0 {
		// Indefinite wait. Discard the tick.
		wait = c.clock.NewTimer(0 * time.Second)
		<-wait.Chan()
	} else {
		wait = c.clock.NewTimer(c.wait)
	}

	actionDone := make(chan struct{})
	var logsWatcher watcher.StringsWatcher
	haveLogs := false
	if len(runningTasks) == 1 {
		var err error
		logsWatcher, err = c.api.WatchActionProgress(ctx, runningTasks[0].task)
		if err != nil {
			return nil, errors.Trace(err)
		}
		processLogMessages(logsWatcher, actionDone, ctx, c.utc, func(ctx *cmd.Context, msg string) {
			haveLogs = true
			c.logMessageHandler(ctx, msg)
		})
	}

	waitForWatcher := func() {
		close(actionDone)
		if logsWatcher != nil {
			_ = logsWatcher.Wait()
		}
	}

	failed := make(map[string]int)
	resultReceivers := set.NewStrings()
	var forceColor, noColor bool
	if _, ok := os.LookupEnv("NO_COLOR"); (ok || os.Getenv("TERM") == "dumb") || c.noColor {
		noColor = true
	}

	if isTerminal(ctx.Stdout) && !noColor || isTerminal(ctx.Stdout) && c.color {
		forceColor = true
	}

	if !isTerminal(ctx.Stdout) && c.color {
		forceColor = true
	}

	for i, result := range runningTasks {
		if forceColor {
			c.progressf(ctx, "Waiting for task %v...\n", colorVal(output.InfoHighlight, result.task))
		} else {
			c.progressf(ctx, "Waiting for task %v...\n", result.task)
		}
		actionResult, err := GetActionResult(ctx, c.api, result.task, c.clock, wait)
		if i == 0 {
			waitForWatcher()
			if haveLogs {
				// Make the logs a bit separate in the output.
				c.progressf(ctx, "\n")
			}
		}
		if err != nil {
			if errors.Is(err, errors.Timeout) {
				return nil, c.handleTimeout(runningTasks, resultReceivers)
			}
			return nil, errors.Trace(err)
		}

		resultReceivers.Add(result.receiver)
		resultData, resultExitCode := formatActionResult(result.task, actionResult, c.utc)
		resultData["id"] = result.task // Action ID is required in case we timed out.
		info[result.receiverId()] = resultData

		// If any of the actions have a error exit code, then inform the user
		// that their exec failed.
		if resultExitCode < 1 {
			continue
		}

		failed[result.task] = resultExitCode
	}

	return failed, c.out.Write(ctx, info)
}

func (c *runCommandBase) handleTimeout(tasks []enqueuedAction, got set.Strings) error {
	want := set.NewStrings()
	for _, t := range tasks {
		want.Add(t.receiver)
	}
	timedOut := want.Difference(got)
	var receivers []string
	for _, r := range timedOut.SortedValues() {
		tag, err := names.ParseTag(r)
		if err != nil {
			continue
		}
		receivers = append(receivers, names.ReadableString(tag))
	}
	return errors.Errorf("timed out waiting for results from: %v", strings.Join(receivers, ", "))
}

// progressf prints progress information such as:
//
//	"Running operation 1 with 2 tasks"
//	"Waiting for task 3..."
//
// This output is sent to either logs or console as per this table:
//
//	 c.hideProgress = |  true   |  false  |
//	------------------|---------|---------|
//	    --quiet flag  |  logs   |  logs   |
//	    neither flag  |  logs   | console |
//	  --verbose flag  | console | console |
//
// By setting the hideProgress field, commands can choose whether these
// messages are logged or sent to console by default.
func (c *runCommandBase) progressf(ctx *cmd.Context, format string, params ...interface{}) {
	if c.hideProgress {
		ctx.Verbosef(format, params...)
	} else {
		ctx.Infof(format, params...)
	}
}

func (c *runCommandBase) printRunOutput(writer io.Writer, value interface{}) error {
	if c.noColor {
		if _, ok := os.LookupEnv("NO_COLOR"); !ok {
			defer os.Unsetenv("NO_COLOR")
			os.Setenv("NO_COLOR", "")
		}
	}

	return printPlainOutput(writer, c.color, value)
}

func (c *runCommandBase) formatYaml(writer io.Writer, value interface{}) error {

	if _, ok := os.LookupEnv("NO_COLOR"); (ok || os.Getenv("TERM") == "dumb") && !c.color || c.noColor {
		return cmd.FormatYaml(writer, value)
	}

	if c.color {
		return output.FormatYamlWithColor(writer, value)
	}

	if isTerminal(writer) && !c.noColor {
		return output.FormatYamlWithColor(writer, value)
	}

	if !isTerminal(writer) && c.color {
		return output.FormatYamlWithColor(writer, value)
	}

	return cmd.FormatYaml(writer, value)
}

func (c *runCommandBase) formatJson(writer io.Writer, value interface{}) error {

	if _, ok := os.LookupEnv("NO_COLOR"); (ok || os.Getenv("TERM") == "dumb") && !c.color || c.noColor {
		return cmd.FormatJson(writer, value)
	}

	if c.color {
		return output.FormatJsonWithColor(writer, value)
	}

	if isTerminal(writer) && !c.noColor {
		return output.FormatJsonWithColor(writer, value)
	}

	if !isTerminal(writer) && c.color {
		return output.FormatJsonWithColor(writer, value)
	}

	return cmd.FormatJson(writer, value)
}

// GetActionResult tries to repeatedly fetch a task until it is
// in a completed state and then it returns it.
// It waits for a maximum of "wait" before returning with the latest action status.
func GetActionResult(ctx context.Context, api APIClient, requestedId string, clk clock.Clock, wait clock.Timer) (actionapi.ActionResult, error) {
	var (
		result actionapi.ActionResult
		err    error
	)
	startTime := clk.Now()
	retryTime := resultPollMinTime
	tick := clk.NewTimer(retryTime)

	// Loop over results until we get "failed" or "completed".  Wait for
	// timer, and reset it each time.
	for {
		result, err = fetchResult(ctx, api, requestedId)
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
		logger.Debugf("after %s action was still %v, will wait %s more before next check",
			clk.Now().Sub(startTime), result.Status, retryTime)

		// Block until a tick happens, or the wait arrives.
		select {
		case <-wait.Chan():
			switch result.Status {
			case params.ActionRunning, params.ActionPending:
				return result, errors.NewTimeout(err, "maximum wait time reached")
			default:
				return result, nil
			}
		case <-tick.Chan():
			// TODO: (jam) 2024-08-29 We could try to reconcile this with either gopkg.in/retry.v1 or
			//  github.com/juju/retry, but neither of them do a great job of handling an exponential
			//  backoff (with max) and a concurrent global max timeout. Maybe gopkg.in/retry.v1.StartWithCancel
			nextRetryTime := time.Duration(float64(retryTime) * resultPollBackoffFactor)
			if nextRetryTime > resultPollMaxTime {
				nextRetryTime = resultPollMaxTime
			}
			retryTime = nextRetryTime
			tick.Reset(retryTime)
		}
	}
}

// fetchResult queries the given API for the given Action ID, and
// makes sure the results are acceptable, returning an error if they are not.
func fetchResult(ctx context.Context, api APIClient, requestedId string) (actionapi.ActionResult, error) {
	none := actionapi.ActionResult{}

	actions, err := api.Actions(ctx, []string{requestedId})
	if err != nil {
		return none, err
	}
	numActionResults := len(actions)
	if numActionResults == 0 {
		return none, errors.NotFoundf("task %v", requestedId)
	}
	if numActionResults != 1 {
		return none, errors.Errorf("too many results for task %s", requestedId)
	}

	result := actions[0]
	if result.Error != nil {
		return none, result.Error
	}

	return result, nil
}

// colorVal appends ansi color codes to the given value
func colorVal(ctx *ansiterm.Context, val interface{}) string {
	buff := &bytes.Buffer{}
	coloredWriter := ansiterm.NewWriter(buff)
	coloredWriter.SetColorCapable(true)

	ctx.Fprint(coloredWriter, val)
	str := buff.String()
	buff.Reset()
	return str
}

// isTerminal checks if the file descriptor is a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}

	return isatty.IsTerminal(f.Fd())
}

type enqueuedAction struct {
	task     string
	receiver string
}

func (a *enqueuedAction) receiverId() string {
	tag, err := names.ParseTag(a.receiver)
	if err != nil {
		return a.receiver
	}
	return tag.Id()
}

func (a *enqueuedAction) GoString() string {
	tag, err := names.ParseTag(a.receiver)
	if err != nil {
		return a.receiver
	}
	return tag.Kind() + " " + tag.Id()
}

// filteredOutputKeys are those we don't want to display as part of the
// results map for plain output.
var filteredOutputKeys = set.NewStrings("return-code", "stdout", "stderr", "stdout-encoding", "stderr-encoding")

// invoked by showtask.go
func printOutput(writer io.Writer, value interface{}) error {
	return printPlainOutput(writer, false, value)
}

func printPlainOutput(writer io.Writer, forceColor bool, value interface{}) error {
	info, ok := value.(map[string]interface{})
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", info, value)
	}

	w := output.PrintWriter{Writer: output.Writer(writer)}
	if forceColor {
		w.SetColorCapable(forceColor)
	}

	// actionInfo contains relevant information for each action result.
	var actionInfo = make(map[string]map[string]interface{})

	/*
		Parse action YAML data that looks like this:

		mysql/0:
		  id: "1"
		  results:
		    <action data here>
		  status: completed
	*/
	var resultMetadata map[string]interface{}
	var stdout, stderr string
	for k := range info {
		resultMetadata, ok = info[k].(map[string]interface{})
		if !ok {
			return errors.Errorf("expected value of type %T, got %T", resultMetadata, info[k])
		}
		resultData, ok := resultMetadata["results"].(map[string]interface{})
		var output string
		if ok {
			resultDataCopy := make(map[string]interface{})
			for k, v := range resultData {
				k = strings.ToLower(k)
				if k == "stdout" && v != "" {
					stdout = fmt.Sprint(v)
				}
				if k == "stderr" && v != "" {
					stderr = fmt.Sprint(v)
				}
				if !filteredOutputKeys.Contains(k) {
					resultDataCopy[k] = v
				}
			}
			if len(resultDataCopy) > 0 {
				data, err := yaml.Marshal(resultDataCopy)
				if err == nil {
					output = string(data)
				} else {
					output = fmt.Sprintf("%v", resultDataCopy)
				}
			}
		} else {
			status, ok := resultMetadata["status"].(string)
			if !ok {
				status = "has unknown status"
			}
			output = fmt.Sprintf("Task %v %v\n", resultMetadata["id"], status)
		}
		actionInfo[k] = map[string]interface{}{
			"id":     resultMetadata["id"],
			"output": output,
			"status": resultMetadata["status"],
		}
		if msg, ok := resultMetadata["message"]; ok {
			actionInfo[k]["message"] = msg
		}
	}
	if len(actionInfo) > 1 {
		return cmd.FormatYaml(writer, actionInfo)
	}
	for _, info := range actionInfo {
		if info["status"] == params.ActionFailed {
			w.Printf(output.ErrorHighlight, "Action id %v failed: %v\n", info["id"], info["message"])
			w.Println(output.ErrorHighlight, info["output"])
		} else {
			w.Println(output.GoodHighlight, info["output"])
		}
	}
	if stdout != "" {
		fmt.Fprintln(writer, strings.Trim(stdout, "\n"))
	}
	if stderr != "" {
		fmt.Fprintln(writer, strings.Trim(stderr, "\n"))
	}
	return nil
}

// formatActionResult removes empty values from the given ActionResult and
// inserts the remaining ones in a map[string]interface{} for cmd.Output to
// write in an easy-to-read format.
func formatActionResult(id string, result actionapi.ActionResult, utc bool) (map[string]interface{}, int) {
	response := map[string]interface{}{"id": id, "status": result.Status}
	if result.Error != nil {
		response["error"] = result.Error.Error()
	}
	if result.Action != nil {
		rt, err := names.ParseTag(result.Action.Receiver)
		if err == nil {
			response[rt.Kind()] = rt.Id()
		}
	}
	if result.Message != "" {
		response["message"] = result.Message
	}
	output, exitCode := convertActionOutput(result.Output)
	if len(result.Output) != 0 {
		response["results"] = output
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

	if result.Enqueued.IsZero() && result.Started.IsZero() && result.Completed.IsZero() {
		return response, exitCode
	}

	responseTiming := make(map[string]string)
	for k, v := range map[string]string{
		"enqueued":  formatTimestamp(result.Enqueued, false, utc, false),
		"started":   formatTimestamp(result.Started, false, utc, false),
		"completed": formatTimestamp(result.Completed, false, utc, false),
	} {
		if v != "" {
			responseTiming[k] = v
		}
	}
	response["timing"] = responseTiming

	return response, exitCode
}

// convertActionOutput returns result data with stdout, stderr etc correctly formatted.
func convertActionOutput(output map[string]interface{}) (map[string]interface{}, int) {
	if output == nil {
		return nil, -1
	}
	values := output
	// We always want to have a string for stdout, but only show stderr,
	// code and error if they are there.
	res, ok := output["stdout"].(string)
	if ok && len(res) > 0 {
		values["stdout"] = strings.Replace(res, "\r\n", "\n", -1)
	} else {
		delete(values, "stdout")
	}
	res, ok = output["stderr"].(string)
	if ok && len(res) > 0 {
		values["stderr"] = strings.Replace(res, "\r\n", "\n", -1)
	} else {
		delete(values, "stderr")
	}
	// return-code may come in as a float64 due to serialisation.
	var v interface{}
	if v, ok = output["return-code"]; ok && v != nil {
		res = fmt.Sprintf("%v", v)
	}
	code := -1
	if ok && len(res) > 0 {
		var err error
		if code, err = strconv.Atoi(res); err == nil {
			values["return-code"] = code
		}
	} else {
		delete(values, "return-code")
	}
	return values, code
}

// addValueToMap adds the given value to the map on which the method is run.
// This allows us to merge maps such as {foo: {bar: baz}} and {foo: {baz: faz}}
// into {foo: {bar: baz, baz: faz}}.
func addValueToMap(keys []string, value interface{}, target map[string]interface{}) {
	next := target

	for i := range keys {
		// If we are on last key set or overwrite the val.
		if i == len(keys)-1 {
			next[keys[i]] = value
			break
		}

		if iface, ok := next[keys[i]]; ok {
			switch typed := iface.(type) {
			case map[string]interface{}:
				// If we already had a map inside, keep
				// stepping through.
				next = typed
			default:
				// If we didn't, then overwrite value
				// with a map and iterate with that.
				m := map[string]interface{}{}
				next[keys[i]] = m
				next = m
			}
			continue
		}

		// Otherwise, it wasn't present, so make it and step
		// into.
		m := map[string]interface{}{}
		next[keys[i]] = m
		next = m
	}
}

const (
	watchTimestampFormat  = "15:04:05"
	resultTimestampFormat = "2006-01-02T15:04:05"
)

func decodeLogMessage(encodedMessage string, utc bool) (string, error) {
	var actionMessage actions.ActionMessage
	err := json.Unmarshal([]byte(encodedMessage), &actionMessage)
	if err != nil {
		return "", errors.Trace(err)
	}
	return formatLogMessage(actionMessage, true, utc, true), nil
}

func formatTimestamp(timestamp time.Time, progressFormat, utc, plain bool) string {
	if timestamp.IsZero() {
		return ""
	}
	if utc {
		timestamp = timestamp.UTC()
	} else {
		timestamp = timestamp.Local()
	}
	if !progressFormat && !plain {
		return timestamp.String()
	}
	timestampFormat := resultTimestampFormat
	if progressFormat {
		timestampFormat = watchTimestampFormat
	}
	return timestamp.Format(timestampFormat)
}

func formatLogMessage(actionMessage actions.ActionMessage, progressFormat, utc, plain bool) string {
	return fmt.Sprintf("%v %v", formatTimestamp(actionMessage.Timestamp, progressFormat, utc, plain), actionMessage.Message)
}

// processLogMessages starts a go routine to decode and handle any incoming
// action log messages received via the string watcher.
func processLogMessages(
	w watcher.StringsWatcher, done chan struct{}, ctx *cmd.Context, utc bool, handler func(*cmd.Context, string),
) {
	go func() {
		defer w.Kill()
		for {
			select {
			case <-done:
				return
			case messages, ok := <-w.Changes():
				if !ok {
					return
				}
				for _, msg := range messages {
					logMsg, err := decodeLogMessage(msg, utc)
					if err != nil {
						logger.Warningf("badly formatted action log message: %v\n%v", err, msg)
						continue
					}
					handler(ctx, logMsg)
				}
			}
		}
	}()
}
