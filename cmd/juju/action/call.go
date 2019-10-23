// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
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

func NewCallCommand() cmd.Command {
	return modelcmd.Wrap(&callCommand{
		logMessageHandler: func(ctx *cmd.Context, msg string) {
			fmt.Fprintln(ctx.Stderr, msg)
		},
	})
}

// callCommand enqueues an Action for running on the given unit with given
// params
type callCommand struct {
	ActionCommandBase
	api               APIClient
	unitReceivers     []string
	leaders           map[string]string
	functionName      string
	paramsYAML        cmd.FileVar
	parseStrings      bool
	background        bool
	maxWait           time.Duration
	out               cmd.Output
	args              [][]string
	utc               bool
	logMessageHandler func(*cmd.Context, string)
}

const callDoc = `
Run a charm function for execution on a given unit, with a given set of params.
An ID is returned for use with 'juju show-task <ID>'.

To queue a function to be run in the background without waiting for it to finish,
use the --background option.

To set the maximum time to wait for a function to complete, use the --max-wait option.

By default, the output of a single function will just be that function's stdout.
For multiple functions, each function stdout is printed with the function id.
To see more detailed information about run timings etc, use --format yaml.

Valid unit identifiers are: 
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

If the leader syntax is used, the leader unit for the application will be
resolved before the function is enqueued.

Params are validated according to the charm for the unit's application.  The
valid params can be seen using "juju functions <application> --schema".
Params may be in a yaml file which is passed with the --params option, or they
may be specified by a key.key.key...=value format (see examples below.)

Params given in the CLI invocation will be parsed as YAML unless the
--string-args option is set.  This can be helpful for values such as 'y', which
is a boolean true in YAML.

If --params is passed, along with key.key...=value explicit arguments, the
explicit arguments will override the parameter file.

Examples:

    juju call mysql/3 backup --background
    juju call mysql/3 backup --max-wait=2m
    juju call mysql/3 backup --format yaml
    juju call mysql/3 backup --utc
    juju call mysql/3 backup
    juju call mysql/leader backup
    juju show-task <ID>
    juju call mysql/3 backup --params parameters.yml
    juju call mysql/3 backup out=out.tar.bz2 file.kind=xz file.quality=high
    juju call mysql/3 backup --params p.yml file.kind=xz file.quality=high
    juju call sleeper/0 pause time=1000
    juju call sleeper/0 pause --string-args time=1000

See also:
    list-tasks
    show-task
`

// SetFlags offers an option for YAML output.
func (c *callCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	c.out.AddFlags(f, "plain", map[string]cmd.Formatter{
		"yaml":  cmd.FormatYaml,
		"json":  cmd.FormatJson,
		"plain": printPlainOutput,
	})

	f.Var(&c.paramsYAML, "params", "Path to yaml-formatted params file")
	f.BoolVar(&c.parseStrings, "string-args", false, "Use raw string values of CLI args")
	f.BoolVar(&c.background, "background", false, "Run the function in the background")
	f.DurationVar(&c.maxWait, "max-wait", 0, "Maximum wait time for a function to complete")
	f.BoolVar(&c.utc, "utc", false, "Show times in UTC")
}

func (c *callCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "call",
		Args:    "<unit> [<unit> ...] <action name> [key.key.key...=value]",
		Purpose: "Run a function on a specified unit.",
		Doc:     callDoc,
	})
}

// Init gets the unit tag(s), action name and action arguments.
func (c *callCommand) Init(args []string) (err error) {
	for _, arg := range args {
		if names.IsValidUnit(arg) || validLeader.MatchString(arg) {
			c.unitReceivers = append(c.unitReceivers, arg)
		} else if nameRule.MatchString(arg) {
			c.functionName = arg
			break
		} else {
			return errors.Errorf("invalid unit or function name %q", arg)
		}
	}
	if len(c.unitReceivers) == 0 {
		return errors.New("no unit specified")
	}
	if c.functionName == "" {
		return errors.New("no function specified")
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

func (c *callCommand) Run(ctx *cmd.Context) error {
	if err := c.ensureAPI(); err != nil {
		return errors.Trace(err)
	}
	defer c.api.Close()

	results, err := c.enqueueActions(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	var actionTag names.ActionTag
	info := make(map[string]interface{}, len(results.Results))
	for _, result := range results.Results {
		if result.Error != nil {
			return result.Error
		}
		if result.Action == nil {
			return errors.Errorf("task failed to enqueue on %q", result.Action.Receiver)
		}
		if actionTag, err = names.ParseActionTag(result.Action.Tag); err != nil {
			return err
		}

		if !c.background {
			ctx.Infof("Running Task %s", actionTag.Id())
		}
		unitTag, err := names.ParseUnitTag(result.Action.Receiver)
		if err != nil {
			return err
		}
		info[unitTag.Id()] = map[string]string{
			"id": actionTag.Id(),
		}
	}
	if c.background {
		if len(results.Results) == 1 {
			ctx.Infof("Scheduled Task %s", actionTag.Id())
			ctx.Infof("Check status with 'juju show-task %s'", actionTag.Id())
		} else {
			ctx.Infof("Scheduled Tasks:")
			cmd.FormatYaml(ctx.Stderr, info)
			ctx.Infof("Check status with 'juju show-task <id>'")
		}
		return nil
	}

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
	if len(results.Results) == 1 && c.api.BestAPIVersion() >= 5 {
		logsWatcher, err = c.api.WatchActionProgress(actionTag.Id())
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
			logsWatcher.Wait()
		}
	}

	for i, result := range results.Results {
		tag, err := names.ParseActionTag(result.Action.Tag)
		if err != nil {
			waitForWatcher()
			return errors.Trace(err)
		}
		fmt.Fprintf(ctx.Stderr, "Waiting for task %v...\n", tag.Id())
		result, err = GetActionResult(c.api, tag.Id(), wait)
		if i == 0 {
			waitForWatcher()
			if haveLogs {
				// Make the logs a bit separate in the output.
				fmt.Fprintln(ctx.Stderr, "")
			}
		}
		if err != nil {
			return errors.Trace(err)
		}
		unitTag, err := names.ParseUnitTag(result.Action.Receiver)
		if err != nil {
			return err
		}
		d := FormatActionResult(result, c.utc)
		d["id"] = tag.Id() // Action ID is required in case we timed out.
		info[unitTag.Id()] = d
	}

	return c.out.Write(ctx, info)
}

func (c *callCommand) enqueueActions(ctx *cmd.Context) (*params.ActionResults, error) {
	actionParams := map[string]interface{}{}
	if c.paramsYAML.Path != "" {
		b, err := c.paramsYAML.Read(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}

		err = yaml.Unmarshal(b, &actionParams)
		if err != nil {
			return nil, errors.Trace(err)
		}

		conformantParams, err := common.ConformYAML(actionParams)
		if err != nil {
			return nil, errors.Trace(err)
		}

		betterParams, ok := conformantParams.(map[string]interface{})
		if !ok {
			return nil, errors.New("params must contain a YAML map with string keys")
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
				return nil, errors.Trace(err)
			}
		}
		// Insert the value in the map.
		addValueToMap(keys, cleansedValue, actionParams)
	}
	conformantParams, err := common.ConformYAML(actionParams)
	if err != nil {
		return nil, errors.Trace(err)
	}
	typedConformantParams, ok := conformantParams.(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("params must be a map, got %T", typedConformantParams)
	}
	actions := make([]params.Action, len(c.unitReceivers))
	for i, unitReceiver := range c.unitReceivers {
		if strings.HasSuffix(unitReceiver, "leader") {
			if c.api.BestAPIVersion() < 3 {
				app := strings.Split(unitReceiver, "/")[0]
				return nil, errors.Errorf("unable to determine leader for application %q"+
					"\nleader determination is unsupported by this API"+
					"\neither upgrade your controller, or explicitly specify a unit", app)
			}
			actions[i].Receiver = unitReceiver
		} else {
			actions[i].Receiver = names.NewUnitTag(unitReceiver).String()
		}
		actions[i].Name = c.functionName
		actions[i].Parameters = actionParams
	}
	results, err := c.api.Enqueue(params.Actions{Actions: actions})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(c.unitReceivers) {
		return nil, errors.New("illegal number of results returned")
	}
	return &results, nil
}

// filteredOutputKeys are those we don't want to display as part of the
// results map for plain output.
var filteredOutputKeys = set.NewStrings("Code", "Stdout", "Stderr", "StdoutEncoding", "StderrEncoding")

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
				if k == "Stdout" && v != "" {
					stdout = fmt.Sprint(v)
				}
				if k == "Stderr" && v != "" {
					stderr = fmt.Sprint(v)
				}
				if !filteredOutputKeys.Contains(k) {
					resultDataCopy[k] = v
				}
			}
			data, err := yaml.Marshal(resultDataCopy)
			if err == nil {
				actionOutput[k] = string(data)
			} else {
				actionOutput[k] = fmt.Sprintf("%v", resultDataCopy)
			}
		} else {
			actionOutput[k] = fmt.Sprintf("Task %v complete\n", resultMetadata["id"])
		}
		actionInfo[k] = map[string]interface{}{
			"id":     resultMetadata["id"],
			"output": actionOutput[k],
		}
	}
	if len(actionOutput) != 1 {
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

func (c *callCommand) ensureAPI() (err error) {
	if c.api != nil {
		return nil
	}
	c.api, err = c.NewActionAPIClient()
	return errors.Trace(err)
}
