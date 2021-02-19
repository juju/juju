// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/juju/charm/v9"
	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/utils/set"
	"gopkg.in/yaml.v2"

	actionapi "github.com/juju/juju/api/action"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/actions"
	coreactions "github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/watcher"
)

var logger = loggo.GetLogger("juju.cmd.juju.action")

// leaderSnippet is a regular expression for unit ID-like syntax that is used
// to indicate the current leader for an application.
const leaderSnippet = "(" + names.ApplicationSnippet + ")/leader"

var validLeader = regexp.MustCompile("^" + leaderSnippet + "$")

// nameRule describes the name format of an action or keyName must match to be valid.
var nameRule = charm.GetActionNameRule()

//resultPollTime is how often to poll the backend for results.
var resultPollTime = 2 * time.Second

type runCommandBase struct {
	ActionCommandBase
	api        APIClient
	background bool
	out        cmd.Output
	utc        bool

	clock       clock.Clock
	wait        time.Duration
	defaultWait time.Duration

	logMessageHandler func(*cmd.Context, string)
}

// SetFlags offers an option for YAML output.
func (c *runCommandBase) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	c.out.AddFlags(f, "plain", map[string]cmd.Formatter{
		"yaml":  cmd.FormatYaml,
		"json":  cmd.FormatJson,
		"plain": printPlainOutput,
	})

	f.BoolVar(&c.background, "background", false, "Run the task in the background")
	f.DurationVar(&c.wait, "wait", 0, "Maximum wait time for a task to complete")
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

func (c *runCommandBase) ensureAPI() (err error) {
	if c.api != nil {
		return nil
	}
	c.api, err = c.NewActionAPIClient()
	return errors.Trace(err)
}

func (c *runCommandBase) processOperationResults(ctx *cmd.Context, results *actionapi.EnqueuedActions) error {
	tasks := make([]enqueuedAction, len(results.Actions))
	for i, a := range results.Actions {
		if a.Error != nil {
			tasks[i].err = a.Error
			continue
		}
		tasks[i] = enqueuedAction{
			task:     a.Action.ID,
			receiver: a.Action.Receiver,
		}
	}
	operationID := results.OperationID
	numTasks := len(tasks)
	if !c.background {
		var plural string
		if numTasks > 1 {
			plural = "s"
		}
		ctx.Infof("Running operation %s with %d task%s", operationID, numTasks, plural)
	}

	var actionID string
	info := make(map[string]interface{}, numTasks)
	for _, result := range tasks {
		if result.err != nil {
			return result.err
		}
		if result.task == "" {
			return errors.Errorf("operation failed to enqueue on %q", result.receiver)
		}
		actionID = result.task

		if !c.background {
			ctx.Infof("  - task %s on %s", actionID, result.receiver)
		}
		info[result.receiverId()] = map[string]string{
			"id": result.task,
		}
	}
	ctx.Infof("")
	if c.background {
		if numTasks == 1 {
			ctx.Infof("Scheduled operation %s with task %s", operationID, actionID)
			ctx.Infof("Check operation status with 'juju show-operation %s'", operationID)
			ctx.Infof("Check task status with 'juju show-task %s'", actionID)
		} else {
			ctx.Infof("Scheduled operation %s with %d tasks", operationID, numTasks)
			_ = cmd.FormatYaml(ctx.Stdout, info)
			ctx.Infof("Check operation status with 'juju show-operation %s'", operationID)
			ctx.Infof("Check task status with 'juju show-task <id>'")
		}
		return nil
	}
	return c.waitForTasks(ctx, tasks, info)
}

func (c *runCommandBase) waitForTasks(ctx *cmd.Context, tasks []enqueuedAction, info map[string]interface{}) error {
	var wait clock.Timer
	if c.wait < 0 {
		// Indefinite wait. Discard the tick.
		wait = c.clock.NewTimer(0 * time.Second)
		_ = <-wait.Chan()
	} else {
		wait = c.clock.NewTimer(c.wait)
	}

	actionDone := make(chan struct{})
	var logsWatcher watcher.StringsWatcher
	haveLogs := false
	if len(tasks) == 1 {
		var err error
		logsWatcher, err = c.api.WatchActionProgress(tasks[0].task)
		if err != nil {
			return errors.Trace(err)
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

	resultReceivers := set.NewStrings()
	for i, result := range tasks {
		ctx.Infof("Waiting for task %v...\n", result.task)
		// tick every two seconds, to delay the loop timer.
		// TODO(fwereade): 2016-03-17 lp:1558657
		tick := c.clock.NewTimer(resultPollTime)
		actionResult, err := GetActionResult(c.api, result.task, tick, wait)
		if i == 0 {
			waitForWatcher()
			if haveLogs {
				// Make the logs a bit separate in the output.
				ctx.Infof("\n")
			}
		}
		if err != nil {
			if errors.IsTimeout(err) {
				return c.handleTimeout(tasks, resultReceivers)
			}
			return errors.Trace(err)
		}

		resultReceivers.Add(result.receiver)
		d := formatActionResult(result.task, actionResult, c.utc)
		d["id"] = result.task // Action ID is required in case we timed out.
		info[result.receiverId()] = d
	}

	return c.out.Write(ctx, info)
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

// GetActionResult tries to repeatedly fetch a task until it is
// in a completed state and then it returns it.
// It waits for a maximum of "wait" before returning with the latest action status.
func GetActionResult(api APIClient, requestedId string, tick, wait clock.Timer) (actionapi.ActionResult, error) {
	return timerLoop(api, requestedId, tick, wait)
}

// timerLoop loops indefinitely to query the given API, until "wait" times
// out, using the "tick" timer to delay the API queries.  It writes the
// result to the given output.
func timerLoop(api APIClient, requestedId string, tick, wait clock.Timer) (actionapi.ActionResult, error) {
	var (
		result actionapi.ActionResult
		err    error
	)

	// Loop over results until we get "failed" or "completed".  Wait for
	// timer, and reset it each time.
	for {
		result, err = fetchResult(api, requestedId)
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

		// Block until a tick happens, or the wait arrives.
		select {
		case _ = <-wait.Chan():
			switch result.Status {
			case params.ActionRunning, params.ActionPending:
				return result, errors.NewTimeout(err, "maximum wait time reached")
			default:
				return result, nil
			}
		case _ = <-tick.Chan():
			tick.Reset(resultPollTime)
		}
	}
}

// fetchResult queries the given API for the given Action ID, and
// makes sure the results are acceptable, returning an error if they are not.
func fetchResult(api APIClient, requestedId string) (actionapi.ActionResult, error) {
	none := actionapi.ActionResult{}

	actions, err := api.Actions([]string{requestedId})
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

type enqueuedAction struct {
	task     string
	receiver string
	err      error
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

func printPlainOutput(writer io.Writer, value interface{}) error {
	info, ok := value.(map[string]interface{})
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", info, value)
	}

	// actionOutput contains the action-set data of each action result.
	// If there's only one action result, just that data is printed.
	var actionOutput = make(map[string]string)

	// actionInfo contains the id and stdout of each action result.
	// It will be printed if there's more than one action result.
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
					actionOutput[k] = string(data)
				} else {
					actionOutput[k] = fmt.Sprintf("%v", resultDataCopy)
				}
			}
		} else {
			status, ok := resultMetadata["status"].(string)
			if !ok {
				status = "has unknown status"
			}
			actionOutput[k] = fmt.Sprintf("Task %v %v\n", resultMetadata["id"], status)
		}
		actionInfo[k] = map[string]interface{}{
			"id":     resultMetadata["id"],
			"output": actionOutput[k],
		}
	}
	if len(actionOutput) > 1 {
		return cmd.FormatYaml(writer, actionInfo)
	}
	for _, msg := range actionOutput {
		fmt.Fprintln(writer, msg)
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
func formatActionResult(id string, result actionapi.ActionResult, utc bool) map[string]interface{} {
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
	output := convertActionOutput(result.Output)
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
		return response
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

	return response
}

// convertActionOutput returns result data with stdout, stderr etc correctly formatted.
func convertActionOutput(output map[string]interface{}) map[string]interface{} {
	if output == nil {
		return nil
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
	if ok && len(res) > 0 {
		code, err := strconv.Atoi(res)
		if err == nil {
			values["return-code"] = code
		}
	} else {
		delete(values, "return-code")
	}
	return values
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
	var actionMessage coreactions.ActionMessage
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

func formatLogMessage(actionMessage coreactions.ActionMessage, progressFormat, utc, plain bool) string {
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
