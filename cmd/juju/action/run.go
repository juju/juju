// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"gopkg.in/yaml.v2"

	actionapi "github.com/juju/juju/api/client/action"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/watcher"
)

// leaderSnippet is a regular expression for unit ID-like syntax that is used
// to indicate the current leader for an application.
const leaderSnippet = "(" + names.ApplicationSnippet + ")/leader"

var validLeader = regexp.MustCompile("^" + leaderSnippet + "$")

// nameRule describes the name format of an action or keyName must match to be valid.
var nameRule = charm.GetActionNameRule()

func NewRunCommand() cmd.Command {
	return modelcmd.Wrap(&runCommand{
		logMessageHandler: func(ctx *cmd.Context, msg string) {
			ctx.Infof("%s", msg)
		},
	})
}

// runCommand enqueues an Action for running on the given unit with given
// params
type runCommand struct {
	ActionCommandBase
	api               APIClient
	unitReceivers     []string
	actionName        string
	paramsYAML        cmd.FileVar
	parseStrings      bool
	background        bool
	maxWait           time.Duration
	out               cmd.Output
	args              [][]string
	utc               bool
	logMessageHandler func(*cmd.Context, string)
}

const runDoc = `
Run a charm action for execution on the given unit(s), with a given set of params.
An ID is returned for use with 'juju show-operation <ID>'.

A action executed on a given unit becomes a task with an ID that can be
used with 'juju show-task <ID>'.

Running an action returns the overall operation ID as well as the individual
task ID(s) for each unit.

To queue a action to be run in the background without waiting for it to finish,
use the --background option.

To set the maximum time to wait for a action to complete, use the --max-wait option.

By default, the output of a single action will just be that action's stdout.
For multiple actions, each action stdout is printed with the action id.
To see more detailed information about run timings etc, use --format yaml.

Valid unit identifiers are: 
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

If the leader syntax is used, the leader unit for the application will be
resolved before the action is enqueued.

Params are validated according to the charm for the unit's application.  The
valid params can be seen using "juju actions <application> --schema".
Params may be in a yaml file which is passed with the --params option, or they
may be specified by a key.key.key...=value format (see examples below.)

Params given in the CLI invocation will be parsed as YAML unless the
--string-args option is set.  This can be helpful for values such as 'y', which
is a boolean true in YAML.

If --params is passed, along with key.key...=value explicit arguments, the
explicit arguments will override the parameter file.

Examples:

    juju run mysql/3 backup --background
    juju run mysql/3 backup --max-wait=2m
    juju run mysql/3 backup --format yaml
    juju run mysql/3 backup --utc
    juju run mysql/3 backup
    juju run mysql/leader backup
    juju show-operation <ID>
    juju run mysql/3 backup --params parameters.yml
    juju run mysql/3 backup out=out.tar.bz2 file.kind=xz file.quality=high
    juju run mysql/3 backup --params p.yml file.kind=xz file.quality=high
    juju run sleeper/0 pause time=1000
    juju run sleeper/0 pause --string-args time=1000

See also:
    list-operations
    list-tasks
    show-operation
    show-task
`

// SetFlags offers an option for YAML output.
func (c *runCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	c.out.AddFlags(f, "plain", map[string]cmd.Formatter{
		"yaml":  cmd.FormatYaml,
		"json":  cmd.FormatJson,
		"plain": printPlainOutput,
	})

	f.Var(&c.paramsYAML, "params", "Path to yaml-formatted params file")
	f.BoolVar(&c.parseStrings, "string-args", false, "Use raw string values of CLI args")
	f.BoolVar(&c.background, "background", false, "Run the action in the background")
	f.DurationVar(&c.maxWait, "max-wait", 0, "Maximum wait time for a action to complete")
	f.BoolVar(&c.utc, "utc", false, "Show times in UTC")
}

func (c *runCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "run",
		Args:    "<unit> [<unit> ...] <action-name> [<key>=<value> [<key>[.<key> ...]=<value>]]",
		Purpose: "Run a action on a specified unit.",
		Doc:     runDoc,
	})
}

// Init gets the unit tag(s), action name and action arguments.
func (c *runCommand) Init(args []string) (err error) {
	for _, arg := range args {
		if names.IsValidUnit(arg) || validLeader.MatchString(arg) {
			c.unitReceivers = append(c.unitReceivers, arg)
		} else if nameRule.MatchString(arg) {
			c.actionName = arg
			break
		} else {
			return errors.Errorf("invalid unit or action name %q", arg)
		}
	}
	if len(c.unitReceivers) == 0 {
		return errors.New("no unit specified")
	}
	if c.actionName == "" {
		return errors.New("no action specified")
	}

	if c.background && c.maxWait > 0 {
		return errors.New("cannot specify both --max-wait and --background")
	}
	if !c.background && c.maxWait == 0 {
		c.maxWait = 60 * time.Second
	}

	// Parse CLI key-value args if they exist.
	c.args = make([][]string, 0)
	for _, arg := range args[len(c.unitReceivers)+1:] {
		thisArg := strings.SplitN(arg, "=", 2)
		if len(thisArg) != 2 {
			return errors.Errorf("argument %q must be of the form key.key.key...=value", arg)
		}
		keySlice := strings.Split(thisArg[0], ".")
		// check each key for validity
		for _, key := range keySlice {
			if valid := nameRule.MatchString(key); !valid {
				return errors.Errorf("key %q must start and end with lowercase alphanumeric, "+
					"and contain only lowercase alphanumeric and hyphens", key)
			}
		}
		c.args = append(c.args, append(keySlice, thisArg[1]))
	}
	return nil
}

func (c *runCommand) Run(ctx *cmd.Context) error {
	if err := c.ensureAPI(); err != nil {
		return errors.Trace(err)
	}
	defer c.api.Close()

	// juju run action is behind a feature flag so we are
	// free to not support running against an older controller
	if c.api.BestAPIVersion() < 6 {
		return errors.Errorf("juju run action not supported on this version of Juju")
	}

	operationID, results, err := c.enqueueActions(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	numTasks := len(results)
	if !c.background {
		var plural string
		if numTasks > 1 {
			plural = "s"
		}
		ctx.Infof("Running operation %s with %d task%s", operationID, numTasks, plural)
	}

	var actionID string
	info := make(map[string]interface{}, numTasks)
	for i, result := range results {
		if result.err != nil {
			return result.err
		}
		if result.task == "" {
			return errors.Errorf("operation failed to enqueue on %q", result.receiver)
		}
		actionID = result.task

		if !c.background {
			ctx.Infof("  - task %s on %s", result.task, c.unitReceivers[i])
		}
		info[result.receiver] = map[string]string{
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
	return c.waitForTasks(ctx, results, info)
}

func (c *runCommand) waitForTasks(ctx *cmd.Context, tasks []enqueuedAction, info map[string]interface{}) error {
	var wait *time.Timer
	if c.maxWait < 0 {
		// Indefinite wait. Discard the tick.
		wait = time.NewTimer(0 * time.Second)
		_ = <-wait.C
	} else {
		wait = time.NewTimer(c.maxWait)
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

	for i, result := range tasks {
		ctx.Infof("Waiting for task %v...\n", result.task)
		actionResult, err := GetActionResult(c.api, result.task, wait, false)
		if i == 0 {
			waitForWatcher()
			if haveLogs {
				// Make the logs a bit separate in the output.
				ctx.Infof("\n")
			}
		}
		if err != nil {
			return errors.Trace(err)
		}
		d := FormatActionResult(result.task, actionResult, c.utc, false)
		d["id"] = result.task // Action ID is required in case we timed out.
		info[result.receiver] = d
	}

	return c.out.Write(ctx, info)
}

type enqueuedAction struct {
	task     string
	receiver string
	err      error
}

func (c *runCommand) enqueueActions(ctx *cmd.Context) (string, []enqueuedAction, error) {
	actionParams := map[string]interface{}{}
	if c.paramsYAML.Path != "" {
		b, err := c.paramsYAML.Read(ctx)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		err = yaml.Unmarshal(b, &actionParams)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		conformantParams, err := common.ConformYAML(actionParams)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		betterParams, ok := conformantParams.(map[string]interface{})
		if !ok {
			return "", nil, errors.New("params must contain a YAML map with string keys")
		}

		actionParams = betterParams
	}
	// If we had explicit args {..., [key, key, key, key, value], ...}
	// then iterate and set params ..., key.key.key.key=value, ...
	for _, argSlice := range c.args {
		valueIndex := len(argSlice) - 1
		keys := argSlice[:valueIndex]
		value := argSlice[valueIndex]
		cleansedValue := interface{}(value)
		if !c.parseStrings {
			err := yaml.Unmarshal([]byte(value), &cleansedValue)
			if err != nil {
				return "", nil, errors.Trace(err)
			}
		}
		// Insert the value in the map.
		addValueToMap(keys, cleansedValue, actionParams)
	}
	conformantParams, err := common.ConformYAML(actionParams)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	typedConformantParams, ok := conformantParams.(map[string]interface{})
	if !ok {
		return "", nil, errors.Errorf("params must be a map, got %T", typedConformantParams)
	}
	actions := make([]actionapi.Action, len(c.unitReceivers))
	for i, unitReceiver := range c.unitReceivers {
		if strings.HasSuffix(unitReceiver, "leader") {
			if c.api.BestAPIVersion() < 3 {
				app := strings.Split(unitReceiver, "/")[0]
				return "", nil, errors.Errorf("unable to determine leader for application %q"+
					"\nleader determination is unsupported by this API"+
					"\neither upgrade your controller, or explicitly specify a unit", app)
			}
			actions[i].Receiver = unitReceiver
		} else {
			actions[i].Receiver = names.NewUnitTag(unitReceiver).String()
		}
		actions[i].Name = c.actionName
		actions[i].Parameters = actionParams
	}
	results, err := c.api.EnqueueOperation(actions)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	if len(results.Actions) != len(c.unitReceivers) {
		return "", nil, errors.New("illegal number of results returned")
	}
	tasks := make([]enqueuedAction, len(results.Actions))
	for i, a := range results.Actions {
		if a.Error != nil {
			tasks[i].err = a.Error
			continue
		}
		tasks[i] = enqueuedAction{
			task:     a.Action.ID,
			receiver: c.unitReceivers[i],
		}
	}
	return results.OperationID, tasks, nil
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
		  id: f47ac10b-58cc-4372-a567-0e02b2c3d479
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

func (c *runCommand) ensureAPI() (err error) {
	if c.api != nil {
		return nil
	}
	c.api, err = c.NewActionAPIClient()
	return errors.Trace(err)
}
