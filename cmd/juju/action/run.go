// Copyright 2014-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"regexp"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// leaderSnippet is a regular expression for unit ID-like syntax that is used
// to indicate the current leader for an application.
const leaderSnippet = "(" + names.ApplicationSnippet + ")/leader"

var validLeader = regexp.MustCompile("^" + leaderSnippet + "$")

// nameRule describes the name format of an action or keyName must match to be valid.
var nameRule = charm.GetActionNameRule()

func NewRunCommand() cmd.Command {
	return modelcmd.Wrap(&runCommand{})
}

// runCommand enqueues an Action for running on the given unit with given
// params
type runCommand struct {
	ActionCommandBase
	api           APIClient
	unitReceivers []string
	leaders       map[string]string
	actionName    string
	paramsYAML    cmd.FileVar
	parseStrings  bool
	wait          waitFlag
	out           cmd.Output
	args          [][]string
}

const runDoc = `
Queue an action for execution on a given unit, with a given set of params.

Valid unit identifiers are:

  - <application-name>/<unit-number>, such as mysql/0 or postgresql/4
  - <application-name>/leader, such as mysql/leader

If the leader syntax is used, the leader unit for the application will be
resolved before the action is enqueued.

Many actions take parameters. Params can be supplied as a YAML file or as 
arguments on the command line. Specify a file with --params <params.yaml>.
Arguments directly on the command line use a <key>=<value> syntax. Nested keys 
require dots as a delimiter: <key1>.<key2>.<key3>=<value> (See examples below)

By default, the action's ID is returned immediately for later use with 
'juju show-action-output <id>' and 'juju show-action-status <id>'. To wait 
for the action to complete, provide the --wait option.

Params given in the CLI invocation will be parsed as YAML unless the
--string-args option is set.  This can be helpful for values such as 'y' and 
'no', which evaluate to boolean values in YAML.

Params provided on the command line override params defined in the file
provided by --params.

Parameters are validated before the action is taken. Use the 
"juju actions <application-name> --schema" command to view the valid 
inputs for the action.

Examples:

Enqueue the "backup" action on unit "mysql/3":

	juju run-action mysql/3 backup

Enqueue the "backup" action on unit "mysql/3", and wait until when the action 
has completed before returning:

    juju run-action mysql/3 backup --wait
    
Specify the current leader unit of the "mysql" application:

    juju run-action mysql/leader backup

Provide parameters to the action:

    juju run-action mysql/3 backup --params parameters.yml
    juju run-action mysql/3 backup out=out.tar.bz2 file.kind=xz file.quality=high
    juju run-action mysql/3 backup --params p.yml file.kind=xz file.quality=high

Provide a param that will be interpreted as an integer:

    juju run-action sleeper/0 pause time=1000

Provide a param that will be interpreted as a string:

    juju run-action sleeper/0 pause --string-args time=1000

Related Commands:
	actions
	show-action-output
	show-action-status

Further Reading:
	 https://docs.jujucharms.com/working-with-actions 
`

// SetFlags offers an option for YAML output.
func (c *runCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
	f.Var(&c.paramsYAML, "params", "Path to YAML-formatted params file")
	f.BoolVar(&c.parseStrings, "string-args", false, "Use raw string values of CLI args")
	f.Var(&c.wait, "wait", "Wait for results, with optional timeout")
}

func (c *runCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "run-action",
		Args:    "<unit> [<unit> ...] <action-name> [key[.key]=value]",
		Purpose: "Queue an action for execution.",
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

	// Parse CLI key-value args if they exist.
	c.args = make([][]string, 0)
	for _, arg := range args[len(c.unitReceivers)+1:] {
		thisArg := strings.SplitN(arg, "=", 2)
		if len(thisArg) != 2 {
			return errors.Errorf("argument %q must be of the form key[.key]=value", arg)
		}
		keySlice := strings.Split(thisArg[0], ".")
		// check each key for validity
		for _, key := range keySlice {
			if valid := nameRule.MatchString(key); !valid {
				return errors.Errorf("key %q must start and end with lowercase alphanumeric, "+
					"and contain only lowercase alphanumeric and hyphens", key)
			}
		}
		// c.args={..., [key, key, key, key, value]}
		c.args = append(c.args, append(keySlice, thisArg[1]))
	}
	return nil
}

func (c *runCommand) Run(ctx *cmd.Context) error {
	if err := c.ensureAPI(); err != nil {
		return errors.Trace(err)
	}
	defer c.api.Close()

	actionParams := map[string]interface{}{}
	if c.paramsYAML.Path != "" {
		b, err := c.paramsYAML.Read(ctx)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(b, &actionParams)
		if err != nil {
			return err
		}

		conformantParams, err := common.ConformYAML(actionParams)
		if err != nil {
			return err
		}

		betterParams, ok := conformantParams.(map[string]interface{})
		if !ok {
			return errors.New("params must contain a YAML map with string keys")
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
				return err
			}
		}
		// Insert the value in the map.
		addValueToMap(keys, cleansedValue, actionParams)
	}

	conformantParams, err := common.ConformYAML(actionParams)
	if err != nil {
		return err
	}

	typedConformantParams, ok := conformantParams.(map[string]interface{})
	if !ok {
		return errors.Errorf("params must be a map, got %T", typedConformantParams)
	}

	actions := make([]params.Action, len(c.unitReceivers))
	for i, unitReceiver := range c.unitReceivers {
		if strings.HasSuffix(unitReceiver, "leader") {
			if c.api.BestAPIVersion() < 3 {
				app := strings.Split(unitReceiver, "/")[0]
				return errors.Errorf("unable to determine leader for application %q"+
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
	results, err := c.api.Enqueue(params.Actions{Actions: actions})
	if err != nil {
		return err
	}

	if len(results.Results) != len(c.unitReceivers) {
		return errors.New("illegal number of results returned")
	}

	for _, result := range results.Results {
		if result.Error != nil {
			return result.Error
		}
		if result.Action == nil {
			return errors.Errorf("action failed to enqueue on %q", result.Action.Receiver)
		}
		tag, err := names.ParseActionTag(result.Action.Tag)
		if err != nil {
			return err
		}

		// Legacy Juju 1.25 output format for a single unit, no wait.
		if !c.wait.forever && c.wait.d.Nanoseconds() <= 0 && len(results.Results) == 1 {
			out := map[string]string{"Action queued with id": tag.Id()}
			return c.out.Write(ctx, out)
		}
	}

	out := make(map[string]interface{}, len(results.Results))

	// Immediate return. This is the default, although rarely
	// what cli users want. We should consider changing this
	// default with Juju 3.0.
	if !c.wait.forever && c.wait.d.Nanoseconds() <= 0 {
		for _, result := range results.Results {
			out[result.Action.Receiver] = result.Action.Tag
			actionTag, err := names.ParseActionTag(result.Action.Tag)
			if err != nil {
				return err
			}
			unitTag, err := names.ParseUnitTag(result.Action.Receiver)
			if err != nil {
				return err
			}
			out[result.Action.Receiver] = map[string]string{
				"id":   actionTag.Id(),
				"unit": unitTag.Id(),
			}
		}
		return c.out.Write(ctx, out)
	}

	var wait *time.Timer
	if c.wait.d.Nanoseconds() <= 0 {
		// Indefinite wait. Discard the tick.
		wait = time.NewTimer(0 * time.Second)
		_ = <-wait.C
	} else {
		wait = time.NewTimer(c.wait.d)
	}

	for _, result := range results.Results {
		tag, err := names.ParseActionTag(result.Action.Tag)
		if err != nil {
			return err
		}
		result, err = GetActionResult(c.api, tag.Id(), wait)
		if err != nil {
			return errors.Trace(err)
		}
		unitTag, err := names.ParseUnitTag(result.Action.Receiver)
		if err != nil {
			return err
		}
		d := FormatActionResult(result)
		d["id"] = tag.Id()       // Action ID is required in case we timed out.
		d["unit"] = unitTag.Id() // Formatted unit is nice to have.
		out[result.Action.Receiver] = d
	}
	return c.out.Write(ctx, out)
}

func (c *runCommand) ensureAPI() (err error) {
	if c.api != nil {
		return nil
	}
	c.api, err = c.NewActionAPIClient()
	return errors.Trace(err)
}
